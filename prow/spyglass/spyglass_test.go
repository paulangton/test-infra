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
	"os"
	"path"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
)

var (
	fakeGCSBucket    *storage.BucketHandle
	testAf           *GCSArtifactFetcher
	fakeGCSJobSource *GCSJobSource
	buildLogName     = path.Join(exampleCIJobPath, "build-log.txt")
	startedName      = path.Join(exampleCIJobPath, "started.json")
	finishedName     = path.Join(exampleCIJobPath, "finished.json")
)

const (
	exampleCIJobPath = "logs/example-ci-run/403"
	testBucketName   = "test-bucket"
)

func TestMain(m *testing.M) {
	fakeGCSServer := fakestorage.NewServer([]fakestorage.Object{
		{
			BucketName: testBucketName,
			Name:       path.Join(exampleCIJobPath, "build-log.txt"),
			Content:    []byte("Oh wow\nlogs\nthis is\ncrazy"),
		},
		{
			BucketName: testBucketName,
			Name:       path.Join(exampleCIJobPath, "started.json"),
			Content: []byte(`{
						  "node": "gke-prow-default-pool-3c8994a8-qfhg", 
						  "repo-version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "timestamp": 1528742858, 
						  "repos": {
						    "k8s.io/kubernetes": "master", 
						    "k8s.io/release": "master"
						  }, 
						  "version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "metadata": {
						    "pod": "cbc53d8e-6da7-11e8-a4ff-0a580a6c0269"
						  }
						}`),
		},
		{
			BucketName: testBucketName,
			Name:       path.Join(exampleCIJobPath, "finished.json"),
			Content: []byte(`{
						  "timestamp": 1528742943, 
						  "version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "result": "SUCCESS", 
						  "passed": true, 
						  "job-version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "metadata": {
						    "repo": "k8s.io/kubernetes", 
						    "repos": {
						      "k8s.io/kubernetes": "master", 
						      "k8s.io/release": "master"
						    }, 
						    "infra-commit": "260081852", 
						    "pod": "cbc53d8e-6da7-11e8-a4ff-0a580a6c0269", 
						    "repo-commit": "e6f64d0a79243c834babda494151fc5d66582240"
						  },
						},`),
		},
	})
	defer fakeGCSServer.Stop()
	fakeGCSJobSource = NewGCSJobSourceWithPrefix("localhost:8080", testBucketName, exampleCIJobPath)
	fakeGCSClient := fakeGCSServer.Client()
	fakeGCSBucket = fakeGCSClient.Bucket(testBucketName)
	testAf = &GCSArtifactFetcher{
		client: fakeGCSClient,
	}
	os.Exit(m.Run())
}