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

package kubeapi

import (
	"log"

	"github.com/coreos/coreos-cloudinit/datasource"
	"github.com/coreos/coreos-cloudinit/pkg"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
)

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

type kubeClient struct {
	clientset       *kubernetes.Clientset
	recorder        record.EventRecorder
	secretNamespace string
	secretName      string
	userDataPath    string
}

func NewDatasource(kubeconfig, namespace, secret, userDataPath string) *kubeClient {
	clientset := pkg.NewKubeClient(kubeconfig)
	recorder := pkg.NewEventRecorder(clientset)
	return &kubeClient{
		clientset:       clientset,
		recorder:        recorder,
		secretNamespace: namespace,
		secretName:      secret,
		userDataPath:    userDataPath,
	}
}

func (k *kubeClient) IsAvailable() bool {
	secret, err := pkg.GetSecret(k.clientset, k.secretNamespace, k.secretName)
	if err != nil {
		log.Println(err.Error())
		return false
	}
	if _, ok := secret.Data[k.userDataPath]; ok {
		return true
	}
	log.Printf("No such path(%s) in the secret(%s) in namespace(%s)!\n", k.userDataPath, k.secretName, k.secretNamespace)
	return false
}

func (k *kubeClient) AvailabilityChanges() bool {
	return true
}

func (k *kubeClient) ConfigRoot() string {
	return ""
}

func (k *kubeClient) FetchMetadata() (datasource.Metadata, error) {
	return datasource.Metadata{}, nil
}

func (k *kubeClient) FetchUserdata() ([]byte, error) {
	secret, err := pkg.GetSecret(k.clientset, k.secretNamespace, k.secretName)
	if err != nil {
		return []byte{}, err
	}
	userData, ok := secret.Data[k.userDataPath]
	if !ok {
		log.Printf("Could not fing key \"%s\" in the secret!\n", k.userDataPath)
		return []byte{}, nil
	}

	// var userDataDecoded []byte
	// _, err = base64.StdEncoding.Decode(userDataDecoded, userDataEncoded)
	// if err != nil {
	// 	return []byte{}, err
	// }

	return userData, nil
}

func (k *kubeClient) Type() string {
	return "kubernetes"
}

func (k *kubeClient) LogEvent(severity datasource.Severity, msg string) {
	var eventType string
	switch severity {
	case datasource.DEBUG, datasource.INFO:
		eventType = apiv1.EventTypeNormal
	case datasource.WARNING, datasource.ERROR, datasource.FATAL:
		eventType = apiv1.EventTypeWarning
	default:
		eventType = apiv1.EventTypeWarning
	}
	pkg.EmmitEvent(k.clientset, k.recorder, eventType, "CloudInit"+string(severity), msg)
	//pkg.MakeEvent(k.clientset, string(severity), "default", "CloudInit"+string(severity), msg)
}
