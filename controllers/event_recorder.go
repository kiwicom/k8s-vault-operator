package controllers

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

type EventRecorder struct {
	Recorder record.EventRecorder
}

func (er *EventRecorder) Normal(obj runtime.Object, reason, msg string) {
	er.Recorder.Event(obj, v1.EventTypeNormal, reason, msg)
}

func (er *EventRecorder) Warning(obj runtime.Object, reason string, err error) {
	er.Recorder.Event(obj, v1.EventTypeWarning, reason, err.Error())
}
