/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spyglass

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/viewers"
	"k8s.io/test-infra/testgrid/util/gcs"
)

const (
	httpScheme  = "http"
	httpsScheme = "https"
)

// GCSArtifactFetcher contains information used for fetching artifacts from GCS
type GCSArtifactFetcher struct {
	client      *storage.Client
	xmlEndpoint string
	withTLS     bool
}

// gcsJobSource is a location in GCS where Prow job-specific artifacts are stored. This implementation assumes
// Prow's native GCS upload format (treating GCS keys as a directory structure), and is not
// intended to support arbitrary GCS bucket upload formats.
type gcsJobSource struct {
	source     string
	linkPrefix string
	bucket     string
	jobPrefix  string
	jobName    string
	buildID    string
}

// NewGCSArtifactFetcher creates a new ArtifactFetcher with a real GCS Client
func NewGCSArtifactFetcher(c *storage.Client, xmlEndpoint string, tls bool) *GCSArtifactFetcher {
	return &GCSArtifactFetcher{
		client:      c,
		xmlEndpoint: xmlEndpoint,
		withTLS:     tls,
	}
}

func fieldsForJob(src jobSource) logrus.Fields {
	return logrus.Fields{
		"jobPrefix": src.jobPath(),
	}
}

// newGCSJobSource creates a new gcsJobSource from a given bucket and jobPrefix
func newGCSJobSource(src string) *gcsJobSource {
	linkPrefix := "gs://"
	noPrefixSrc := strings.TrimPrefix(src, linkPrefix)
	if !strings.HasSuffix(noPrefixSrc, "/") { // Cleaning up path
		noPrefixSrc += "/"
	}
	tokens := strings.FieldsFunc(noPrefixSrc, func(c rune) bool { return c == '/' })
	bucket := tokens[0]
	buildID := tokens[len(tokens)-1]
	name := tokens[len(tokens)-2]
	jobPrefix := strings.TrimPrefix(noPrefixSrc, bucket+"/") // Extra / is not part of prefix, only necessary for URI
	return &gcsJobSource{
		source:     src,
		linkPrefix: linkPrefix,
		bucket:     bucket,
		jobPrefix:  jobPrefix,
		jobName:    name,
		buildID:    buildID,
	}
}

// GCSMarker holds the starting point for the next paginated GCS query
type GCSMarker struct {
	XMLName xml.Name `xml:"ListBucketResult"`
	Marker  string   `xml:"NextMarker"`
}

// Contents is a single entry returned by the GCS XML API
type Contents struct {
	Key string
}

// GCSReq contains the contents of a GCS XML API list response
type GCSReq struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []Contents
}

// names is a helper function for extracting artifact names in parallel from a GCS XML API response
func names(content []byte, names chan<- string, src jobSource, wg *sync.WaitGroup) {
	defer wg.Done()
	extracted := GCSReq{
		Contents: []Contents{},
	}
	err := xml.Unmarshal(content, &extracted)
	if err != nil {
		logrus.WithFields(fieldsForJob(src)).WithError(err).Error("Error unmarshaling artifact names from XML")
	}
	for _, c := range extracted.Contents {
		names <- c.Key
	}
}

// tryGCSXMLGet is a helper function that attempts to list the artifacts available for the given job
// source with the given scheme and number of retries. It returns whatever it was able to get, even if
// it runs out of retries.
func tryGCSXMLGet(src jobSource, scheme string, endpoint string, retries int, maxResults int) [][]byte {
	bucketName, prefix := extractBucketPrefixPair(src.jobPath())
	bodies := [][]byte{}
	marker := GCSMarker{}
	c := http.Client{}
	if scheme != httpsScheme {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		c = http.Client{Transport: tr}
		scheme = httpScheme
	}
	retry := 0
	for retry < retries {
		params := url.Values{}
		params.Add("prefix", prefix)
		if marker.Marker != "" {
			params.Add("marker", marker.Marker)
		}
		req := url.URL{
			Scheme:   scheme,
			Host:     endpoint,
			Path:     bucketName,
			RawQuery: params.Encode(),
		}
		resp, err := c.Get(req.String())
		if err != nil {
			logrus.WithField("request", req.String()).WithFields(fieldsForJob(src)).WithError(err).Error("Error in GCS XML API GET request")
			retry++
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logrus.WithField("request", req.String()).WithFields(fieldsForJob(src)).WithError(err).Error("Error reading body of GCS XML API response")
			retry++
			continue
		}
		bodies = append(bodies, body)

		marker = GCSMarker{}
		err = xml.Unmarshal(body, &marker)
		if err != nil {
			logrus.WithField("request", req.String()).WithFields(fieldsForJob(src)).WithError(err).Error("Error unmarshaling body of GCS XML API response")
		}
		if marker.Marker == "" {
			break
		}
	}
	return bodies
}

// Artifacts lists all artifacts available for the given job source.
// Uses the GCS XML API because it is ~2x faster than the golang GCS library
// for large number of artifacts. It should also be S3 compatible according to
// the GCS api docs.
func (af *GCSArtifactFetcher) artifacts(src jobSource) []string {
	var wg sync.WaitGroup
	artStart := time.Now()
	artifacts := []string{}
	var scheme string
	if af.withTLS {
		scheme = httpsScheme
	} else {
		scheme = httpScheme
	}
	maxResults := 1000

	gcsSrc := newGCSJobSource(fmt.Sprintf("gs://%s", src.jobPath()))

	bodies := tryGCSXMLGet(gcsSrc, scheme, af.xmlEndpoint, 5, maxResults)

	namesChan := make(chan string, maxResults*len(bodies))
	for _, body := range bodies {
		wg.Add(1)
		go names(body, namesChan, src, &wg)
	}

	wg.Wait()
	close(namesChan)
	for name := range namesChan {
		aName := strings.TrimPrefix(name, gcsSrc.jobPrefix)
		artifacts = append(artifacts, aName)
	}
	artElapsed := time.Since(artStart)
	logrus.WithField("duration", artElapsed).Infof("Listed %d GCS artifacts.", len(artifacts))
	return artifacts
}

// Artifact contructs a GCS artifact from the given GCS bucket and key. Uses the golang GCS library
// to get read handles. If the artifactName is not a valid key in the bucket a handle will still be
// constructed and returned, but all read operations will fail (dictated by behavior of golang GCS lib).
func (af *GCSArtifactFetcher) artifact(src jobSource, artifactName string, sizeLimit int64) viewers.Artifact {
	bucketName, prefix := extractBucketPrefixPair(src.jobPath())
	bkt := af.client.Bucket(bucketName)
	obj := bkt.Object(path.Join(prefix, artifactName))
	artifactLink := &url.URL{
		Scheme: httpsScheme,
		Host:   "storage.googleapis.com",
		Path:   path.Join(src.jobPath(), artifactName),
	}
	return NewGCSArtifact(context.Background(), obj, artifactLink.String(), artifactName, sizeLimit)
}

func extractBucketPrefixPair(gcsPath string) (string, string) {
	split := strings.SplitN(gcsPath, "/", 2)
	return split[0], split[1]
}

// createJobSource tries to create a GCS job source from the provided string
func (af *GCSArtifactFetcher) createJobSource(src string) (jobSource, error) {
	gcsURL, err := url.Parse(src)
	if err != nil {
		return &gcsJobSource{}, ErrCannotParseSource
	}
	gcsPath := &gcs.Path{}
	err = gcsPath.SetURL(gcsURL)
	if err != nil {
		return &gcsJobSource{}, ErrCannotParseSource
	}
	return newGCSJobSource(gcsPath.String()), nil
}

// CanonicalLink gets a link to the location of job-specific artifacts in GCS
func (src *gcsJobSource) canonicalLink() string {
	return path.Join(src.linkPrefix, src.bucket, src.jobPrefix)
}

// JobPath gets the prefix to all artifacts in GCS in the job
func (src *gcsJobSource) jobPath() string {
	return fmt.Sprintf("%s/%s", src.bucket, src.jobPrefix)
}
