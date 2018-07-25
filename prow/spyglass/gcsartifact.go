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
	"compress/gzip"
	"context"
	"fmt"
	"io/ioutil"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/viewers"
)

// GCSArtifact represents some output of a prow job stored in GCS
type GCSArtifact struct {
	// The handle of the object in GCS
	handle *storage.ObjectHandle

	// The link to the Artifact in GCS
	link string

	// The path of the Artifact within the job
	path string

	// sizeLimit is the max size to read before failing
	sizeLimit int64

	// ctx provides context for cancellation and timeout. Embedded in struct to preserve
	// conformance with io.ReaderAt
	ctx context.Context
}

// NewGCSArtifact returns a new GCSArtifact with a given handle, canonical link, and path within the job
func NewGCSArtifact(ctx context.Context, handle *storage.ObjectHandle, link string, path string, sizeLimit int64) *GCSArtifact {
	return &GCSArtifact{
		handle:    handle,
		link:      link,
		path:      path,
		sizeLimit: sizeLimit,
		ctx:       ctx,
	}
}

func fieldsFor(a *GCSArtifact) logrus.Fields {
	return logrus.Fields{
		"artifact": a.path,
	}
}

// Size returns the size of the artifact in GCS
func (a *GCSArtifact) Size() (int64, error) {
	attrs, err := a.handle.Attrs(a.ctx)
	if err != nil {
		return 0, fmt.Errorf("error getting gcs attributes for artifact: %v", err)
	}
	return attrs.Size, nil
}

// JobPath gets the GCS path of the artifact within the current job
func (a *GCSArtifact) JobPath() string {
	return a.path
}

// CanonicalLink gets the GCS web address of the artifact
func (a *GCSArtifact) CanonicalLink() string {
	return a.link
}

// ReadAt reads len(p) bytes from a file in GCS at offset off
func (a *GCSArtifact) ReadAt(p []byte, off int64) (n int, err error) {
	gzipped, err := a.gzipped()
	if err != nil {
		return 0, fmt.Errorf("error checking artifact for gzip compression: %v", err)
	}
	if gzipped {
		return 0, viewers.ErrGzipOffsetRead
	}
	reader, err := a.handle.NewRangeReader(a.ctx, off, int64(len(p)))
	defer reader.Close()
	if err != nil {
		return 0, fmt.Errorf("error getting artifact reader: %v", err)
	}
	n, err = reader.Read(p)
	if err != nil {
		return 0, fmt.Errorf("error reading from artifact: %v", err)
	}
	return n, nil
}

// ReadAtMost reads at most n bytes from a file in GCS. If the file is compressed (gzip) in GCS, n bytes
// of gzipped content will be downloaded and decompressed into potentially GREATER than n bytes of content.
func (a *GCSArtifact) ReadAtMost(n int64) ([]byte, error) {
	reader, err := a.handle.NewRangeReader(a.ctx, 0, n)
	defer reader.Close()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact reader: %v", err)
	}
	var p []byte
	var e error
	gzipped, err := a.gzipped()
	if err != nil {
		return nil, fmt.Errorf("error checking artifact for gzip compression: %v", err)
	}
	if gzipped {
		gReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("error getting gzip reader: %v", err)
		}
		p, e = ioutil.ReadAll(gReader)
		if e != nil {
			return nil, fmt.Errorf("error reading all from gzipped artifact: %v", err)
		}
	} else {
		p, e = ioutil.ReadAll(reader)
		if e != nil {
			return nil, fmt.Errorf("error reading all from artifact: %v", err)
		}
	}
	return p, nil
}

// ReadAll will either read the entire file or throw an error if file size is too big
func (a *GCSArtifact) ReadAll() ([]byte, error) {
	size, err := a.Size()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact size: %v", err)
	}
	if size > a.sizeLimit {
		return nil, viewers.ErrFileTooLarge
	}
	reader, err := a.handle.NewReader(a.ctx)
	defer reader.Close()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact reader: %v", err)
	}
	p, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading all from artifact: %v", err)
	}
	return p, nil
}

// ReadTail reads the last n bytes from a file in GCS
func (a *GCSArtifact) ReadTail(n int64) ([]byte, error) {
	gzipped, err := a.gzipped()
	if err != nil {
		return nil, fmt.Errorf("error checking artifact for gzip compression: %v", err)
	}
	if gzipped {
		return nil, viewers.ErrGzipOffsetRead
	}
	size, err := a.Size()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact size: %v", err)
	}
	var offset int64
	if n > size {
		offset = 0
	} else {
		offset = size - n
	}
	reader, err := a.handle.NewRangeReader(a.ctx, offset, -1)
	defer reader.Close()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact reader: %v", err)
	}
	read, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading all from artiact: %v", err)
	}
	return read, nil
}

// UseContext sets the context to be used in GCS read operations
func (a *GCSArtifact) UseContext(ctx context.Context) error {
	a.ctx = ctx
	return nil
}

// gzipped returns whether the file is gzip-encoded in GCS
func (a *GCSArtifact) gzipped() (bool, error) {
	attrs, err := a.handle.Attrs(a.ctx)
	if err != nil {
		return false, fmt.Errorf("error getting gcs attributes for artifact: %v", err)
	}
	return attrs.ContentEncoding == "gzip", nil
}
