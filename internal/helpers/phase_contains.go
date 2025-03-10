package helpers

import (
	slacknotifierv1 "github.com/mgufrone/slack-notifier/api/v1"
	v1 "k8s.io/api/core/v1"
)

func PhaseContains(haystack []v1.PodPhase, needle v1.PodPhase) bool {
	for _, phase := range haystack {
		if phase == needle {
			return true
		}
	}
	return false
}

func StatusContains(haystack []slacknotifierv1.Status, needle slacknotifierv1.Status) bool {
	for _, phase := range haystack {
		if phase == needle {
			return true
		}
	}
	return false
}
