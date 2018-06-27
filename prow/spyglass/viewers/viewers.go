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

// Package viewers provides interfaces and methods necessary for implementing views
package viewers

import (
	"encoding/json"
	"io"
)

// Artifact represents some output of a prow job
type Artifact interface {
	io.ReaderAt
	CanonicalLink() string
	JobPath() string
	ReadAll() ([]byte, error)
	ReadTail(n int64) ([]byte, error)
	Size() int64
}

// Viewer generates html views for sets of artifacts
type Viewer interface {
	View(artifacts []Artifact, raw *json.RawMessage) string
	Title() string
	Name() string
}
