package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	appscontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	flcontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/flatnetwork.pandaria.io/v1"
)

type Handler struct {
	ipClient          flcontroller.FlatNetworkIPClient
	subnetClient      flcontroller.FlatNetworkSubnetClient
	podClient         corecontroller.PodClient
	deploymentClient  appscontroller.DeploymentClient
	daemonSetClient   appscontroller.DaemonSetClient
	statefulSetClient appscontroller.StatefulSetClient
	cronJobClient     batchcontroller.CronJobClient
	jobClient         batchcontroller.JobClient
}

func NewWebhookHandler(wctx *wrangler.Context) *Handler {
	return &Handler{
		ipClient:          wctx.FlatNetwork.FlatNetworkIP(),
		subnetClient:      wctx.FlatNetwork.FlatNetworkSubnet(),
		podClient:         wctx.Core.Pod(),
		deploymentClient:  wctx.Apps.Deployment(),
		daemonSetClient:   wctx.Apps.DaemonSet(),
		statefulSetClient: wctx.Apps.StatefulSet(),
		cronJobClient:     wctx.Batch.CronJob(),
		jobClient:         wctx.Batch.Job(),
	}
}

func (h *Handler) ValidateHandler(w http.ResponseWriter, req *http.Request) {
	/* read AdmissionReview from the HTTP request */
	ar, httpStatus, err := readAdmissionReview(req)
	if err != nil {
		http.Error(w, err.Error(), httpStatus)
		return
	}

	allowed, err := h.validateAdmissionReview(ar)

	/* perform actual object validation */
	if err != nil {
		handleValidationError(w, ar, err)
		return
	}
	/* perpare response and send it back to the API server */
	err = prepareAdmissionReviewResponse(allowed, "", ar)
	if err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeResponse(w, ar)
}

func prepareAdmissionReviewResponse(allowed bool, message string, ar *admissionv1.AdmissionReview) error {
	if ar.Request != nil {
		ar.Response = &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: allowed,
		}
		if message != "" {
			ar.Response.Result = &metav1.Status{
				Message: message,
			}
		}
		return nil
	}
	return errors.New("received empty AdmissionReview request")
}

func readAdmissionReview(req *http.Request) (*admissionv1.AdmissionReview, int, error) {
	var body []byte

	if req.Body != nil {
		if data, err := io.ReadAll(req.Body); err == nil {
			body = data
		}
	}

	if len(body) == 0 {
		err := errors.New("Error reading HTTP request: empty body")
		logrus.Error(err)
		return nil, http.StatusBadRequest, err
	}

	/* validate HTTP request headers */
	contentType := req.Header.Get("Content-Type")
	if contentType != "application/json" {
		err := errors.Errorf("Invalid Content-Type='%s', expected 'application/json'", contentType)
		logrus.Error(err)
		return nil, http.StatusUnsupportedMediaType, err
	}

	/* read AdmissionReview from the request body */
	ar, err := deserializeAdmissionReview(body)
	if err != nil {
		err := errors.Wrap(err, "error deserializing AdmissionReview")
		logrus.Error(err)
		return nil, http.StatusBadRequest, err
	}

	return ar, http.StatusOK, nil
}

func deserializeAdmissionReview(body []byte) (*admissionv1.AdmissionReview, error) {
	ar := &admissionv1.AdmissionReview{}
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)
	deserializer := codecs.UniversalDeserializer()
	_, _, err := deserializer.Decode(body, nil, ar)

	/* Decode() won't return an error if the data wasn't actual AdmissionReview */
	if err == nil && ar.TypeMeta.Kind != "AdmissionReview" {
		err = errors.New("received object is not an AdmissionReview")
	}

	return ar, err
}

func handleValidationError(w http.ResponseWriter, ar *admissionv1.AdmissionReview, orgErr error) {
	err := prepareAdmissionReviewResponse(false, orgErr.Error(), ar)
	if err != nil {
		err := fmt.Errorf("failed to prepare AdmissionResponse: %w", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeResponse(w, ar)
}

func writeResponse(w http.ResponseWriter, a any) {
	resp, _ := json.Marshal(a)
	w.Write(resp)
}

const (
	kindDeployment  = "Deployment"
	kindDaemonSet   = "DaemonSet"
	kindStatefulSet = "StatefulSet"
	kindCronJob     = "CronJob"
	kindJob         = "Job"
)

func (h *Handler) validateAdmissionReview(ar *admissionv1.AdmissionReview) (bool, error) {
	logrus.Debugf("webhook validateAdmissionReview:  %s %s %#v %#v",
		ar.Request.Name, ar.Request.Namespace, ar.Request.Kind, ar.Request.Resource)
	var (
		ok  bool
		err error
	)
	switch ar.Request.Kind.Kind {
	case "FlatNetworkSubnet":
		ok, err = h.validateFlatNetworkSubnet(ar)
	case kindDeployment, kindDaemonSet, kindStatefulSet, kindCronJob, kindJob:
		ok, err = h.validateWorkload(ar)
	default:
		return true, nil
	}
	if err != nil {
		logrus.Errorf("failed to validate %v: [%v/%v]: %v",
			ar.Request.Kind.Kind, ar.Request.Namespace, ar.Request.Name, err)
	}
	return ok, err
}
