package kubecontroller

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/coreos/coreos-cloudinit/config"
	"github.com/coreos/coreos-cloudinit/config/validate"
	"github.com/coreos/coreos-cloudinit/datasource"
	"github.com/coreos/coreos-cloudinit/initialize"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const (
	workerQueuName         = "Cloud-Init User-Data Secrets"
	controllerAgentName    = "Kube-Cloud-Init-Controler"
	SuccessApplied         = "Cloud-Init Applied"
	FailApplied            = "Cloud-Init Apply Failure"
	MessageResourceApplied = "Cloud-Init applied successfully"
)

// Controller is the controller implementation for Foo resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface

	secretsLister appslisters.SecretLister

	secretsSynced cache.InformerSynced
	foosLister    listers.FooLister
	foosSynced    cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(kubeclientset kubernetes.Interface, secretInformer appsinformers.SecretInformer) *Controller {

	// Create event broadcaster
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset: kubeclientset,
		secretsLister: secretInformer.Lister(),
		secretsSynced: secretInformer.Informer().HasSynced,
		workqueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Cloud-Init User-Data Secrets"),
		recorder:      recorder,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Foo resources change
	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueSecret,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueSecret(new)
		},
	})

	return controller
}

// enqueueFoo takes a Foo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Foo.
func (c *Controller) enqueueSecret(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			c.workqueue.Forget(obj)
			return fmt.Errorf("error applaying '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Cloud-Init Secret resource with this namespace/name
	secret, err := c.secretsLister.Secrets(namespace).Get(name)
	if err != nil {
		// The Secret resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("secret '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	userdataBytes, ok := secret.Data[k.userDataPath]
	if !ok {
		return fmt.Errorf("could not fing key \"%s\" in the secret %s", k.userDataPath, key)
	}
	userdataBytes, err = decompressIfGzip(userdataBytes)
	if err != nil {
		return fmt.Errorf("failed decompressing user-data from datasource: %v", err)
	}
	if report, err := validate.Validate(userdataBytes); err == nil {
		ret := 0
		for _, e := range report.Entries() {
			klog.V(2).Warning("creating event broadcaster")
			ret = 1
		}
		if flags.validate {
			if ret == 1 {
				return fmt.Errorf("failed while validating user_data")
			}
			return nil
		}
	} else {
		return fmt.Errorf("failed while validating user_data (%q)", err)
	}

	metadata := datasource.Metadata{}
	// Apply environment to user-data
	env := initialize.NewEnvironment("/", ds.ConfigRoot(), flags.workspace, flags.sshKeyName, metadata)
	userdata := env.Apply(string(userdataBytes))

	var ccu *config.CloudConfig
	var script *config.Script
	switch ud, err := initialize.ParseUserData(userdata); err {
	case initialize.ErrIgnitionConfig:
		return fmt.Errorf("detected an Ignition config. Exiting")
	case nil:
		switch t := ud.(type) {
		case *config.CloudConfig:
			ccu = t
		case *config.Script:
			script = t
		}
	default:
		return fmt.Errorf("failed to parse user-data: %v", err)
	}

	if err = initialize.Apply(cc, ifaces, env); err != nil {
		return fmt.Errorf("failed to apply cloud-config: %v", err)
	} else {
		c.recordEvent(corev1.EventTypeNormal, SuccessApplied, MessageResourceApplied)
	}

	if script != nil {
		if err = runScript(*script, env); err != nil {
			return fmt.Errorf("failed to run script: %v", err)
		}
	} else {
		c.recordEvent(corev1.EventTypeNormal, SuccessApplied, MessageResourceApplied)
	}

	return nil
}

const gzipMagicBytes = "\x1f\x8b"

func decompressIfGzip(userdataBytes []byte) ([]byte, error) {
	if !bytes.HasPrefix(userdataBytes, []byte(gzipMagicBytes)) {
		return userdataBytes, nil
	}
	gzr, err := gzip.NewReader(bytes.NewReader(userdataBytes))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()
	return ioutil.ReadAll(gzr)
}

func (c *Controller) recordEvent(eventType, reason, message string) {
	nodeName, err := os.Hostname()
	if err != nil {
		klog.V(2).Warning("Warrning can't determine the node name!")
		return
	}
	nodeName = "shoot--i330716--gcp-seed-worker-qejhl-z1-755fd89877-b4gxt"
	node, err := c.kubeclientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})

	if err != nil {
		klog.V(2).Warningf("Warrning can't retrieve node form cluster: %v!", err)
		return
	}
	node.ObjectMeta.UID = types.UID(nodeName)
	c.recorder.Event(node, eventType, reason, message)
}
