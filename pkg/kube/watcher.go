package kube

import (
	"time"

	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type EventHandler func(event *EnhancedEvent)

type EventChecker func(event *EnhancedEvent) bool

type EventWatcher struct {
	informer        cache.SharedInformer
	stopper         chan struct{}
	labelCache      *LabelCache
	annotationCache *AnnotationCache
	fn              EventHandler
	checker         EventChecker
}

func NewEventWatcher(config *rest.Config, namespace string, fn EventHandler, checker EventChecker) *EventWatcher {
	clientset := kubernetes.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithNamespace(namespace))
	informer := factory.Core().V1().Events().Informer()

	watcher := &EventWatcher{
		informer:        informer,
		stopper:         make(chan struct{}),
		labelCache:      NewLabelCache(config),
		annotationCache: NewAnnotationCache(config),
		fn:              fn,
		checker:         checker,
	}

	informer.AddEventHandler(watcher)

	return watcher
}

func (e *EventWatcher) OnAdd(obj interface{}) {
	event := obj.(*corev1.Event)
	e.onEvent(event)
}

func (e *EventWatcher) OnUpdate(oldObj, newObj interface{}) {
	event := newObj.(*corev1.Event)
	e.onEvent(event)
}

func (e *EventWatcher) onEvent(event *corev1.Event) {
	// TODO: Re-enable this after development
	// It's probably an old event we are catching, it's not the best way but anyways
	if time.Since(event.LastTimestamp.Time) > time.Second*5 {
		return
	}

	log.Debug().
		Str("msg", event.Message).
		Str("namespace", event.Namespace).
		Str("reason", event.Reason).
		Str("involvedObject", event.InvolvedObject.Name).
		Msg("Received event")

	ev := &EnhancedEvent{
		Event: *event.DeepCopy(),
	}
	ev.InvolvedObject.ObjectReference = *event.InvolvedObject.DeepCopy()

	// Check to see if we need to process this event, if we don't then skip processing the event to avoid unnecessary
	// errors (specifically RBAC rules associated with objects we don't have permission to)
	shouldProcess := e.checker(ev)
	if !shouldProcess {
		log.Debug().
			Str("msg", event.Message).
			Str("namespace", event.Namespace).
			Str("reason", event.Reason).
			Str("involvedObject", event.InvolvedObject.Name).
			Msg("No rules matched, stopped processing")
		return
	}

	labels, err := e.labelCache.GetLabelsWithCache(&event.InvolvedObject)
	if err != nil {
		log.Error().Err(err).Msg("Cannot list labels of the object")
		// Ignoring error, but log it anyways
	} else {
		ev.InvolvedObject.Labels = labels
	}

	annotations, err := e.annotationCache.GetAnnotationsWithCache(&event.InvolvedObject)
	if err != nil {
		log.Error().Err(err).Msg("Cannot list annotations of the object")
	} else {
		ev.InvolvedObject.Annotations = annotations
	}

	e.fn(ev)
	return
}

func (e *EventWatcher) OnDelete(obj interface{}) {
	// Ignore deletes
}

func (e *EventWatcher) Start() {
	go e.informer.Run(e.stopper)
}

func (e *EventWatcher) Stop() {
	e.stopper <- struct{}{}
	close(e.stopper)
}
