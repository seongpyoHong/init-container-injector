package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
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
	port                    int    // webhook server port
	certFile                string // path to the x509 certificate for https
	keyFile                 string // path to the x509 private key matching `CertFile`
	initContainerConfigFile string // path to init container injector configuration file
}

type Config struct {
	Containers []corev1.Container `yaml:"containers"`
	Volumes    []corev1.Volume    `yaml:"volumes"`
}

type WebhookServer struct {
	initContainerConfig *Config
	server              *http.Server
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
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
		log.Error("Empty Body")
		http.Error(responseWriter, "Empty Body", http.StatusBadRequest)
	}

	var admissionResponse *admissionv1.AdmissionResponse
	originAdmissionReview := admissionv1.AdmissionReview{}
	if _, _, err := deserializer.Decode(requestBody, nil, &originAdmissionReview); err != nil {
		log.Errorf("Can't decode request body: %v", err)
		admissionResponse = &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = ws.mutate(&originAdmissionReview)
	}

	mutatedAdmissionReview := admissionv1.AdmissionReview{}
	mutatedAdmissionReview.Response = admissionResponse
	if originAdmissionReview.Request != nil {
		mutatedAdmissionReview.Response.UID = originAdmissionReview.Request.UID
	}

	data, err := json.Marshal(mutatedAdmissionReview)
	if err != nil {
		log.Errorf("Can't encode response : %v", err)
		http.Error(responseWriter, fmt.Sprintf("Can't encode response : %v", err), http.StatusInternalServerError)
	}

	log.Infof("Ready to write response")
	if _, err := responseWriter.Write(data); err != nil {
		log.Errorf("Can't Write Response : %v", err)
		http.Error(responseWriter, fmt.Sprintf("Can't write response : %v", err), http.StatusInternalServerError)
	}
}

func (ws WebhookServer) mutate(admissionReview *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	request := admissionReview.Request
	var deployment appv1.Deployment
	if err := json.Unmarshal(request.Object.Raw, &deployment); err != nil {
		log.Errorf("Couldn't unmarshall raw object : %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Infof("AdmissionReview for Kind=%v | Namespace=%v | Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v",
		request.Kind, request.Namespace, request.Name, deployment.Name, request.UID, request.Operation, request.UserInfo)

	if !isMutationTarget(ignoredNamespaces, &deployment.ObjectMeta) {
		log.Infof("Skip mutation for %s/%s", deployment.Namespace, deployment.Namespace)
		return &admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}

	applyDefaultWorkaround(ws.initContainerConfig.Containers)
	annotations := map[string]string{admissionWebhookAnnotationInjectKey: "injected"}
	patchBytes, err := createPatch(&deployment, ws.initContainerConfig, annotations)

	if err != nil {
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Infof("AdmissionResponse JSONPatch = %v\n", string(patchBytes))
	return &admissionv1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

func createPatch(deployment *appv1.Deployment, config *Config, annotations map[string]string) ([]byte, error) {
	var patch []patchOperation
	patch = append(patch, addInitContainer(deployment.Spec.Template.Spec.InitContainers, config.Containers, "/spec/template/spec/initcontainers/")...)
	patch = append(patch, updateAnnotation(deployment.Annotations, annotations)...)

	return json.Marshal(patch)
}

func updateAnnotation(deployAnnotation map[string]string, annotations map[string]string) (patch []patchOperation) {
	for k, v := range annotations {
		if deployAnnotation != nil && deployAnnotation[k] == "true" {
			patch = append(patch, patchOperation{
				Op:    "replace",
				Path:  "/metadata/annotations/" + k,
				Value: v,
			})
		}
	}
	return patch
}

func addInitContainer(deployInitContainers []corev1.Container, initContainers []corev1.Container, basePath string) (patch []patchOperation) {
	isFirstInitContainer := len(deployInitContainers) == 0
	var value interface{}
	for _, add := range initContainers {
		path := basePath
		if isFirstInitContainer {
			isFirstInitContainer = false
			value = []corev1.Container{add}
		} else {
			value = add
			path += "/-"
		}

		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}

	return patch
}

func applyDefaultWorkaround(containers []corev1.Container) {
	defaulter.Default(&corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: containers,
		},
	})
}

func isMutationTarget(ignoreNamespaces []string, metadata *metav1.ObjectMeta) bool {
	for _, namespace := range ignoredNamespaces {
		if metadata.Namespace == namespace {
			log.Infof("Skip mutation for %s namespace")
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
