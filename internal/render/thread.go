package render

import (
	_ "embed"
	slacknotifierv1 "github.com/mgufrone/slack-notifier/api/v1"
	"io"
	v1 "k8s.io/api/core/v1"
	"text/template"
)

//go:embed thread.tmpl
var threadTemplate string

type ThreadData struct {
	Pod            v1.Pod
	Owner          *v1.ObjectReference
	PreviousStatus v1.PodStatus
	Status         slacknotifierv1.Status
	Reason         string
}

func Thread(data ThreadData, writer io.Writer) error {
	tmpl, err := template.New("thread").Parse(threadTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(writer, data)
}
