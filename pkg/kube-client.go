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
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	//utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
)

const charset = "abcdef123456789"

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

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

func NewEventRecorder(kubeclientset *kubernetes.Clientset) record.EventRecorder {
	//utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(log.Printf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, apiv1.EventSource{Component: "cloud-init"})
	return recorder
}

func GetSecret(clientset *kubernetes.Clientset, namespace, secretName string) (*apiv1.Secret, error) {
	return clientset.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
}

func EmmitEvent(kubeclientset *kubernetes.Clientset, recorder record.EventRecorder, eventType, reason, message string) {
	nodeName, err := os.Hostname()
	if err != nil {
		log.Println("Warrning can't determine the node name!")
		return
	}
	nodeName = "shoot--i330716--gcp-seed-worker-qejhl-z1-755fd89877-b4gxt"
	node, err := kubeclientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	// node := &apiv1.Node{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name: nodeName,
	// 		UID:  types.UID(nodeName),
	// 	},
	// }
	if err != nil {
		log.Printf("Warrning can't retrieve node form cluster: %v!", err)
		return
	}
	node.ObjectMeta.UID = types.UID(nodeName)
	recorder.Event(node, eventType, reason, message)
}

func MakeEvent(clientset *kubernetes.Clientset, severity, namespace, reason, msg string) {
	nodeName, err := os.Hostname()
	if err != nil {
		log.Println("Warrning can't determine the node name!")
		return
	}
	//nodeName = "shoot--i330716--gcp-seed-worker-qejhl-z1-755fd89877-22tdn"
	eventClient := clientset.CoreV1().Events(namespace)
	event := &apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getEventName(),
			Namespace: namespace,
		},
		InvolvedObject: apiv1.ObjectReference{
			Kind:      "Node",
			Name:      nodeName,
			Namespace: namespace,
		},
		EventTime:           metav1.MicroTime{time.Now()},
		LastTimestamp:       metav1.Time{time.Now()},
		Reason:              reason,
		Message:             msg,
		ReportingController: "cloud-init",
		ReportingInstance:   "cloud-init",
		Action:              "none",
		Type:                severity,
	}
	result, err := eventClient.Create(event)
	if err != nil {
		log.Printf("Warrning can't create Event %s: %v!\n", result.GetObjectMeta().GetName(), err)
	}
}

func getEventName() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Warrning can't determine the host name!")
		hostname = randomPostFix(20)
	}
	return fmt.Sprintf("%s.%s", hostname, randomPostFix(16))
}

func randomPostFix(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
