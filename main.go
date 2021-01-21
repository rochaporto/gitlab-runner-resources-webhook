package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

func main() {
	var params WebhookParams

	// get command line parameters
	flag.IntVar(&params.port, "port", 443, "Webhook server port.")
	flag.StringVar(&params.certFile, "tlsCertFile", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&params.keyFile, "tlsKeyFile", "/etc/webhook/certs/key.pem", "File containing the x509 private key to --tlsCertFile.")
	flag.Parse()

	pair, err := tls.LoadX509KeyPair(params.certFile, params.keyFile)
	if err != nil {
		fmt.Printf("Failed to load key pair: %v\n", err)
	}

	whsvr := &WebhookServer{
		server: &http.Server{
			Addr:      fmt.Sprintf(":%v", params.port),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
	}

	// define http server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.mutate)
	whsvr.server.Handler = mux

	// start webhook server in new routine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			fmt.Printf("Failed to listen and serve webhook server: %v\n", err)
		}
	}()

	fmt.Printf("Server started. Listening on %v\n", params.port)

	// listening OS shutdown singal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	fmt.Printf("Got OS shutdown signal, shutting down webhook server gracefully...\n")
	whsvr.server.Shutdown(context.Background())
}

type WebhookServer struct {
	server *http.Server
}

// Webhook Server parameters
type WebhookParams struct {
	port     int
	certFile string
	keyFile  string
}

func (wh *WebhookServer) mutate(w http.ResponseWriter, r *http.Request) {

	admReview := &admissionv1.AdmissionReview{}

	// read the AdmissionReview from the request json body
	err := json.NewDecoder(r.Body).Decode(admReview)
	if err != nil {
		fmt.Printf("invalid JSON input\n")
		http.Error(w, fmt.Sprintf("invalid JSON input"), http.StatusInternalServerError)
		return
	}

	// unmarshal the pod from the AdmissionRequest
	pod := &corev1.Pod{}
	if err := json.Unmarshal(admReview.Request.Object.Raw, pod); err != nil {
		fmt.Printf("failed to unmarshal to pod: %v", err)
		http.Error(w, fmt.Sprintf("failed to unmarshal to pod: %v", err), http.StatusInternalServerError)
		return
	}

	// add resources request
	for i := 0; i < len(pod.Spec.Containers); i++ {
		if pod.Spec.Containers[i].Name == "build" {
			pod.Spec.Containers[i].Resources.Limits = corev1.ResourceList{
				"nvidia.com/gpu": resource.MustParse("1"),
			}
		}
	}

	cBytes, err := json.Marshal(&pod.Spec.Containers)
	if err != nil {
		fmt.Printf("failed to marshall container: %v", err)
		http.Error(w, fmt.Sprintf("failed to marshall container: %v", err), http.StatusInternalServerError)
		return
	}

	// build json patch
	patch := []JSONPatchEntry{
		JSONPatchEntry{
			OP:    "replace",
			Path:  "/spec/containers",
			Value: cBytes,
		},
	}

	patchBytes, err := json.Marshal(&patch)
	if err != nil {
		fmt.Printf("failed to marshall jsonpatch: %v\n", err)
		http.Error(w, fmt.Sprintf("failed to marshall jsonpatch: %v", err), http.StatusInternalServerError)
	}

	patchType := admissionv1.PatchTypeJSONPatch

	// build admission response
	admResponse := &admissionv1.AdmissionResponse{
		UID:       admReview.Request.UID,
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &patchType,
	}

	respAdmissionReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: admResponse,
	}

	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(&respAdmissionReview)
	if err != nil {
		http.Error(w, fmt.Sprintf("json encoding error: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(b)
	if err != nil {
		http.Error(w, fmt.Sprintf("write error: %v", err), http.StatusInternalServerError)
		return
	}
}

type JSONPatchEntry struct {
	OP    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
}
