/*
Copyright 2019 Baidu, Inc.

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

package controllermanager

import (
	"encoding/json"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/baidu/ote-stack/pkg/reporter"
)

func (u *UpstreamProcessor) handlePodReport(b []byte) error {
	// Deserialize byte data to PodReportStatus
	prs, err := PodReportStatusDeserialize(b)
	if err != nil {
		return fmt.Errorf("PodReportStatusDeserialize failed : %v", err)
	}
	// handle FullList
	if prs.FullList != nil {
		// TODO:handle full pod resource.
	}
	// handle UpdateMap
	if prs.UpdateMap != nil {
		u.handlePodUpdateMap(prs.UpdateMap)
	}
	// handle DelMap
	if prs.DelMap != nil {
		u.handlePodDelMap(prs.DelMap)
	}

	return nil
}

func (u *UpstreamProcessor) handlePodDelMap(delMap map[string]*corev1.Pod) {
	for _, pod := range delMap {

		err := u.DeletePod(pod)
		if err != nil {
			klog.Errorf("Del pod failed : %v", err)
			continue
		}

		klog.V(3).Infof("Deleted pod : namespace(%s), name(%s)", pod.Namespace, pod.Name)
	}
}

func (u *UpstreamProcessor) handlePodUpdateMap(updateMap map[string]*corev1.Pod) {
	for _, pod := range updateMap {

		err := u.CreateOrUpdatePod(pod)
		if err != nil {
			klog.Errorf("Create or update pod failed : %v", err)
			continue
		}
	}
}

//PodReportStatusDeserialize deserialize byte data to PodReportStatus.
func PodReportStatusDeserialize(b []byte) (*reporter.PodResourceStatus, error) {
	podReportStatus := reporter.PodResourceStatus{}
	err := json.Unmarshal(b, &podReportStatus)
	if err != nil {
		return nil, err
	}
	return &podReportStatus, nil
}

// GetPod will retrieve the requested pod based on namespace and name.
func (u *UpstreamProcessor) GetPod(pod *corev1.Pod) (*corev1.Pod, error) {
	pod, err := u.ctx.K8sClient.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod, err
}

// CreatePod will create the given pod.
func (u *UpstreamProcessor) CreatePod(pod *corev1.Pod) error {
	_, err := u.ctx.K8sClient.CoreV1().Pods(pod.Namespace).Create(pod)
	if err != nil {
		return err
	}

	klog.V(3).Infof("Created pod : namespace(%s), name(%s)", pod.Namespace, pod.Name)

	return nil
}

// UpdatePod will update the given pod.
func (u *UpstreamProcessor) UpdatePod(pod *corev1.Pod) error {
	storedPod, err := u.GetPod(pod)
	if err != nil {
		return err
	}

	err = u.checkEdgeVersion(pod, storedPod)

	if err != nil {
		return err
	}

	pod.ResourceVersion = storedPod.ResourceVersion
	// In the case of concurrency, try again if a conflict occurs
	_, err = u.ctx.K8sClient.CoreV1().Pods(pod.Namespace).Update(pod)

	if err != nil && errors.IsConflict(err) {
		return u.UpdatePod(pod)
	}

	if err != nil {
		return err
	}

	klog.V(3).Infof("Updated pod : namespace(%s), name(%s)", pod.Namespace, pod.Name)

	return nil
}

// CreateOrUpdatePod will update the given pod or create it if does not exist.
func (u *UpstreamProcessor) CreateOrUpdatePod(pod *corev1.Pod) error {
	_, err := u.GetPod(pod)
	// If not found resource, create it.
	if err != nil && errors.IsNotFound(err) {
		return u.CreatePod(pod)
	}

	if err != nil {
		return err
	}

	return u.UpdatePod(pod)
}

func (u *UpstreamProcessor) checkEdgeVersion(pod *corev1.Pod, storedPod *corev1.Pod) error {
	if pod.Labels[reporter.EdgeVersionLabel] != "" && storedPod.Labels[reporter.EdgeVersionLabel] != "" {

		// resource report sequential checking
		podVersion, err := strconv.Atoi(pod.Labels[reporter.EdgeVersionLabel])
		if err != nil {
			return err
		}

		storedPodVersion, err := strconv.Atoi(storedPod.Labels[reporter.EdgeVersionLabel])
		if err != nil {
			return err
		}

		// resource report sequential checking
		if podVersion <= storedPodVersion {
			return fmt.Errorf("Current edge-version(%s) is less than or equal to ETCD's edge-version(%s)",
				pod.Labels[reporter.EdgeVersionLabel], storedPod.Labels[reporter.EdgeVersionLabel])
		}
	}
	return nil
}

// DeletePod will delete the given pod.
func (u *UpstreamProcessor) DeletePod(pod *corev1.Pod) error {
	return u.ctx.K8sClient.CoreV1().Pods(pod.Namespace).Delete(pod.Name, &metav1.DeleteOptions{})
}
