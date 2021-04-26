package main

import (
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	server *http.Server
}

func init() {
	corev1.AddToScheme(runtimeScheme)
	admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	v1.AddToScheme(runtimeScheme)
}

func (ws WebhookServer) serve(responseWrite http.ResponseWriter, request *http.Request){

}