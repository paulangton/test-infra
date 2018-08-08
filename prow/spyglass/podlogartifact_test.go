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
	"fmt"
	"io"
	"testing"

	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

type podLogJAgent struct {
}

func (j *podLogJAgent) GetJobLog(job, id string) ([]byte, error) {
	if job == "BFG" && id == "435" {
		return []byte("frobscottle"), nil
	} else if job == "Fantastic Mr. Fox" && id == "4" {
		return []byte("a hundred smoked hams and fifty sides of bacon"), nil
	}
	return nil, fmt.Errorf("could not find job %s, id %s", job, id)
}

func (j *podLogJAgent) GetJobLogTail(job, id string, n int64) ([]byte, error) {
	log, err := j.GetJobLog(job, id)
	if err != nil {
		return nil, fmt.Errorf("error getting log tail: %v", err)
	}
	logLen := int64(len(log))
	if n > logLen {
		return log, nil
	}
	return log[logLen-n:], nil

}

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
	fakePodLogAgent := &podLogJAgent{}
	notFoundArtifact, err := NewPodLogArtifact("job", "123", "", 500e6, fakePodLogAgent)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	bfgArtifact, err := NewPodLogArtifact("BFG", "435", "", 500e6, fakePodLogAgent)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	foxArtifact, err := NewPodLogArtifact("Fantastic Mr. Fox", "4", "", 500e6, fakePodLogAgent)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	foxLowLimitArtifact, err := NewPodLogArtifact("Fantastic Mr. Fox", "4", "", 5, fakePodLogAgent)
	if err != nil {
		t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
	}
	testCases := []struct {
		name        string
		artifact    *PodLogArtifact
		expectedErr error
		expected    []byte
	}{
		{
			name:        "Podlog readall not found",
			artifact:    notFoundArtifact,
			expectedErr: fmt.Errorf("error getting pod log size: error getting size of pod log: could not find job job, id 123"),
			expected:    []byte(""),
		},
		{
			name:        "\"BFG\" Podlog readall",
			artifact:    bfgArtifact,
			expectedErr: nil,
			expected:    []byte("frobscottle"),
		},
		{
			name:        "\"Fantastic Mr. Fox\" Podlog readall",
			artifact:    foxArtifact,
			expectedErr: nil,
			expected:    []byte("a hundred smoked hams and fifty sides of bacon"),
		},
		{
			name:        "Podlog readall over size limit",
			artifact:    foxLowLimitArtifact,
			expectedErr: viewers.ErrFileTooLarge,
			expected:    []byte(""),
		},
	}
	for _, tc := range testCases {
		res, err := tc.artifact.ReadAll()
		if err != nil && err.Error() != tc.expectedErr.Error() {
			t.Fatalf("%s failed reading bytes of log. got err: %v, expected err: %v", tc.name, err, tc.expectedErr)
		}
		if !bytes.Equal(tc.expected, res) {
			t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
		}

	}

}
