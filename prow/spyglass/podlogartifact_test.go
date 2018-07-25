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
	"bytes"
	"io"
	"testing"

	"k8s.io/test-infra/prow/deck/jobs"
)

func TestNewPodLogArtifact(t *testing.T) {
	testCases := []struct {
		name         string
		jobName      string
		buildID      string
		podName      string
		sizeLimit    int64
		ja           *jobs.JobAgent
		expectedErr  error
		expectedLink string
	}{
		{
			name:         "Create pod log with valid fields",
			jobName:      "job",
			buildID:      "123",
			podName:      "",
			sizeLimit:    500e6,
			ja:           fakeJa,
			expectedErr:  nil,
			expectedLink: "/log?id=123&job=job",
		},
		{
			name:         "Create pod log with no jobName",
			jobName:      "",
			buildID:      "123",
			podName:      "",
			sizeLimit:    500e6,
			ja:           fakeJa,
			expectedErr:  errInsufficientJobInfo,
			expectedLink: "",
		},
		{
			name:         "Create pod log with negative sizeLimit",
			jobName:      "job",
			buildID:      "123",
			podName:      "",
			sizeLimit:    -4,
			ja:           fakeJa,
			expectedErr:  errInvalidSizeLimit,
			expectedLink: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, tc.podName, tc.sizeLimit, tc.ja)
			if err != nil {
				if err != tc.expectedErr {
					t.Fatalf("failed creating artifact. err: %v", err)
				}
				return
			}
			link := artifact.CanonicalLink()
			if link != tc.expectedLink {
				t.Errorf("Unexpected link, expected %s, got %q", tc.expectedLink, link)
			}
		})
	}
}

func TestPodLogReadTail(t *testing.T) {
	jobArtifact, err := NewPodLogArtifact("job", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	jibArtifact, err := NewPodLogArtifact("jib", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	testCases := []struct {
		name     string
		artifact *PodLogArtifact
		n        int64
		expected []byte
	}{
		{
			name:     "\"Job\" Podlog ReadTail longer than contents",
			artifact: jobArtifact,
			n:        50,
			expected: []byte("clusterA"),
		},
		{
			name:     "\"Jib\" Podlog ReadTail shorter than contents",
			artifact: jibArtifact,
			n:        3,
			expected: []byte("erB"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.artifact.ReadTail(tc.n)
			if err != nil {
				t.Fatalf("failed reading bytes of log. err: %v", err)
			}
			res = bytes.Trim(res, "\x00")
			if !bytes.Equal(tc.expected, res) {
				t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
			}
		})
	}

}
func TestPodLogReadAt(t *testing.T) {
	jobArtifact, err := NewPodLogArtifact("job", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	jibArtifact, err := NewPodLogArtifact("jib", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	testCases := []struct {
		name        string
		artifact    *PodLogArtifact
		n           int64
		offset      int64
		expectedErr error
		expected    []byte
	}{
		{
			name:        "\"Job\" Podlog ReadAt range longer than contents",
			artifact:    jobArtifact,
			n:           100,
			offset:      3,
			expectedErr: io.EOF,
			expected:    []byte("sterA"),
		},
		{
			name:        "\"Jib\" Podlog ReadAt range within contents",
			artifact:    jibArtifact,
			n:           4,
			offset:      2,
			expectedErr: nil,
			expected:    []byte("uste"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := make([]byte, tc.n)
			_, err := tc.artifact.ReadAt(res, tc.offset)
			if err != tc.expectedErr {
				t.Fatalf("failed reading bytes of log. err: %v, expected err: %v", err, tc.expectedErr)
			}
			res = bytes.Trim(res, "\x00")
			if !bytes.Equal(tc.expected, res) {
				t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
			}
		})
	}

}
func TestPodLogReadAtMost(t *testing.T) {
	jobArtifact, err := NewPodLogArtifact("job", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	jibArtifact, err := NewPodLogArtifact("jib", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	testCases := []struct {
		name        string
		artifact    *PodLogArtifact
		n           int64
		expectedErr error
		expected    []byte
	}{
		{
			name:        "\"Job\" Podlog ReadAtMost longer than contents",
			artifact:    jobArtifact,
			n:           100,
			expectedErr: io.EOF,
			expected:    []byte("clusterA"),
		},
		{
			name:        "\"Jib\" Podlog ReadAtMost shorter than contents",
			artifact:    jibArtifact,
			n:           3,
			expectedErr: nil,
			expected:    []byte("clu"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.artifact.ReadAtMost(tc.n)
			if err != tc.expectedErr {
				t.Fatalf("failed reading bytes of log. err: %v, expected err: %v", err, tc.expectedErr)
			}
			res = bytes.Trim(res, "\x00")
			if !bytes.Equal(tc.expected, res) {
				t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
			}
		})
	}

}

func TestPodLogReadAll(t *testing.T) {
	jobArtifact, err := NewPodLogArtifact("job", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	jibArtifact, err := NewPodLogArtifact("jib", "123", "", 500e6, fakeJa)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	testCases := []struct {
		name     string
		artifact *PodLogArtifact
		expected []byte
	}{
		{
			name:     "\"Job\" Podlog readall",
			artifact: jobArtifact,
			expected: []byte("clusterA"),
		},
		{
			name:     "\"Jib\" Podlog readall",
			artifact: jibArtifact,
			expected: []byte("clusterB"),
		},
	}
	for _, tc := range testCases {
		res, err := tc.artifact.ReadAll()
		if err != nil {
			t.Fatalf("%s failed reading bytes of log. err: %s", tc.name, err)
		}
		if !bytes.Equal(tc.expected, res) {
			t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
		}

	}

}
