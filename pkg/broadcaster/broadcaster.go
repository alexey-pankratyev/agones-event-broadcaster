package broadcaster

import (
	"github.com/Octops/agones-event-broadcaster/pkg/brokers"
	"github.com/Octops/agones-event-broadcaster/pkg/controller"
	"github.com/Octops/agones-event-broadcaster/pkg/events"
	"github.com/Octops/agones-event-broadcaster/pkg/manager"
	"github.com/Octops/agones-event-broadcaster/pkg/runtime/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

// Broadcaster receives events (Add, Update and Delete) sent by the controller
// and uses a Broker to publish those events.
type Broadcaster struct {
	logger      *logrus.Entry
	controllers []*controller.AgonesController
	brokers.Broker
	error   error
	Manager *manager.Manager
}

// New returns a new GameServer broadcaster
// It required a config to be passed to the GameServer controller
// and a broker that will be publishing messages
func New(config *rest.Config, broker brokers.Broker, syncPeriod time.Duration) *Broadcaster {
	logger := log.NewLoggerWithField("source", "broadcaster")

	broadcaster := &Broadcaster{
		logger: logger,
		Broker: broker,
	}

	mgr, err := manager.New(config, manager.Options{
		SyncPeriod: &syncPeriod,
	})

	if err != nil {
		broadcaster.error = errors.Wrap(err, "error creating manager")
		return nil
	}

	broadcaster.Manager = mgr

	return broadcaster
}

// WithWatcherFor adds a controller for the specified obj. The controller reports back to the broadcaster events of type
// OnAdd, OnUpdate and OnDelete associated to that particular resource type.
// Examples of obj arguments are: &v1.GameServer and &v1.Fleet
func (b *Broadcaster) WithWatcherFor(obj runtime.Object) *Broadcaster {
	ctrlFor, err := controller.NewAgonesController(b.Manager, b, controller.Options{
		For:  obj,
		Owns: &corev1.Pod{},
	})

	if err != nil {
		return nil
	}

	b.addController(ctrlFor)

	return b
}

// Build will check for required broadcaster components e return error if the requirements are not satisfied
func (b *Broadcaster) Build() error {
	if b.Manager == nil {
		b.error = errors.Wrap(b.error, "broadcaster requires a manager to operate")
	}

	if len(b.controllers) == 0 {
		b.error = errors.Wrap(b.error, "can't build a broadcaster without controllers, use WithController method to add a controller")
	}

	if b.error != nil {
		return b.error
	}

	return nil
}

// Start run the controller that sends events back to the broadcaster event handlers
func (b *Broadcaster) Start() error {
	b.logger.Info("starting broadcaster")
	chDone := ctrl.SetupSignalHandler()
	if err := b.Manager.Start(chDone); err != nil {
		b.logger.Fatal(errors.Wrap(err, "broadcaster could not start"))
	}

	return nil
}

// OnAdd is the event handler that reacts to Add events
func (b *Broadcaster) OnAdd(obj interface{}) error {
	if b.Broker == nil {
		b.logger.Warn("broker is not available for the broadcaster, message will not be published")
		return nil
	}

	message := &events.EventMessage{
		Body: obj,
	}

	event := events.OnAdded(message)

	return b.Publish(event)
}

// OnUpdate is the event handler that reacts to Update events
func (b *Broadcaster) OnUpdate(oldObj interface{}, newObj interface{}) error {
	if b.Broker == nil {
		b.logger.Warn("a broker is not available for the broadcaster, message will not be published")
		return nil
	}

	body := struct {
		OldObj interface{}
		NewObj interface{}
	}{
		OldObj: oldObj,
		NewObj: newObj,
	}

	message := &events.EventMessage{
		Body: body,
	}

	event := events.OnUpdated(message)

	return b.Publish(event)
}

// OnDelete is the event handler that reacts to Delete events
func (b *Broadcaster) OnDelete(obj interface{}) error {
	if b.Broker == nil {
		b.logger.Warn("a broker is not available for the broadcaster, message will not be published")
		return nil
	}

	message := &events.EventMessage{
		Body: obj,
	}

	event := events.OnDeleted(message)

	return b.Publish(event)
}

// Publish will publish the event wrapped on a envelope using the broker available
func (b *Broadcaster) Publish(event events.Event) error {
	envelope, err := b.Broker.BuildEnvelope(event)
	if err != nil {
		b.logger.WithError(err).Error("error building envelope")
		return err
	}

	if err = b.Broker.SendMessage(envelope); err != nil {
		b.logger.WithError(err).Error("error sending envelope")
		return err
	}

	return nil
}

func (b *Broadcaster) addController(controller *controller.AgonesController) {
	b.controllers = append(b.controllers, controller)
}
