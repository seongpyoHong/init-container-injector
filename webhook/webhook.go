package main

import (
	"encoding/json"
	"github.com/golang/glog"
	"io/ioutil"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"net/http"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
	defaulter     = runtime.ObjectDefaulter(runtimeScheme)
)

var ignoredNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
}

const (
	admissionWebhookAnnotationInjectKey = "init-container-injector-webhook.sphong.com/inject"
)

// Webhook Server parameters
type WhSvrParameters struct {
	port                   int    // webhook server port
	certFile               string // path to the x509 certificate for https
	keyFile                string // path to the x509 private key matching `CertFile`
	initConainerConfigFile string // path to init container injector configuration file
}

type Config struct {
	Containers []corev1.Container `yaml:"containers"`
	Volumes    []corev1.Volume    `yaml:"volumes"`
}

type WebhookServer struct {
	initContainerConfig *Config
	server              *http.Server
}

func init() {
	corev1.AddToScheme(runtimeScheme)
	admissionregistrationv1.AddToScheme(runtimeScheme)
	corev1.AddToScheme(runtimeScheme)
}

func (ws WebhookServer) Serve(responseWriter http.ResponseWriter, request *http.Request) {
	var requestBody []byte
	if request.Body != nil {
		if data, err := ioutil.ReadAll(request.Body); err == nil {
			requestBody = data
		}
	}

	if len(requestBody) == 0 {
		glog.Error("Empty Body")
		http.Error(responseWriter, "Empty Body", http.StatusBadRequest)
	}

	var admissionResponse *admissionv1.AdmissionResponse
	admissionReview := admissionv1.AdmissionReview{}
	if _, _, err := deserializer.Decode(requestBody, nil, &admissionReview); err != nil {
		glog.Errorf("Can't decode request body: %v", err)
		admissionResponse = &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = ws.mutate(&admissionReview)
	}
}

func (ws WebhookServer) mutate(admissionReview *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	request := admissionReview.Request
	var deployment appv1.Deployment
	if err := json.Unmarshal(request.Object.Raw, &deployment); err != nil {
		glog.Errorf("Couldn't unmarshall raw object : %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionReview for Kind=%v | Namespace=%v | Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v",
		request.Kind, request.Namespace,request.Name, deployment.Name, request.UID, request.Operation, request.UserInfo)

	if !isMutationTarget(ignoredNamespaces, &deployment.ObjectMeta) {
		glog.Infof("Skip mutation for %s/%s", deployment.Namespace, deployment.Namespace)
		return &admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}

	//TODO: mutating
}

func isMutationTarget(ignoreNamespaces []string, metadata *metav1.ObjectMeta) bool {
	for _, namespace := range ignoredNamespaces {
		if metadata.Namespace == namespace {
			glog.Infof("Skip mutation for %s namespace")
			return false
		}
	}

	annotation := metadata.GetAnnotations()
	if annotation == nil {
		annotation = make(map[string]string)
	}

	status := annotation[admissionWebhookAnnotationInjectKey]
	if strings.ToLower(status) == "yes" {
		return true
	}
	return false
}
