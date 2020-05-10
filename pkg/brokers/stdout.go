package brokers

import (
	"encoding/json"
	"github.com/Octops/gameserver-events-broadcaster/pkg/events"
	"github.com/sirupsen/logrus"
)

type StdoutBroker struct {
}

func (s *StdoutBroker) BuildEnvelope(event events.Event) (*events.Envelope, error) {
	eventType := event.EventType()

	envelope := &events.Envelope{}
	envelope.AddHeader("event_type", eventType)
	envelope.Message = event.(events.Message).Content()

	return envelope, nil
}

func (s *StdoutBroker) SendMessage(envelope *events.Envelope) error {
	output, _ := json.Marshal(envelope)
	logrus.Infof("%s", output)

	return nil
}
