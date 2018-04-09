// Copyright 2015 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
type Datasource interface {
	IsAvailable() bool
	AvailabilityChanges() bool
	ConfigRoot() string
	FetchMetadata() (Metadata, error)
	FetchUserdata() ([]byte, error)
	Type() string
}

type Metadata struct {
	PublicIPv4    net.IP
	PublicIPv6    net.IP
	PrivateIPv4   net.IP
	PrivateIPv6   net.IP
	Hostname      string
	SSHPublicKeys map[string]string
	NetworkConfig interface{}
}
*/

package pkg

import (
	"log"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func NewKubeClient(kubeconfig string) *kubernetes.Clientset {
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Println(err.Error())
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Println(err.Error())
		return nil
	}

	return clientset
}

func GetSecret(clientset *kubernetes.Clientset, namespace, secretName string) (*v1.Secret, error) {
	return clientset.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
}
