/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/mgufrone/slack-notifier/internal/helpers"
	"github.com/mgufrone/slack-notifier/internal/render"
	"github.com/slack-go/slack"
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	slacknotifierv1 "github.com/mgufrone/slack-notifier/api/v1"
)

// SlackChannelReconciler reconciles a SlackChannel object
type SlackChannelReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	slackClient *slack.Client
	cacheClient *ristretto.Cache[string, any]
}

// NewSlackChannelReconciler will create new SlackChannelReconciler struct from given arguments
func NewSlackChannelReconciler(manager ctrl.Manager, slackClient *slack.Client, cacheClient *ristretto.Cache[string, any]) *SlackChannelReconciler {
	return &SlackChannelReconciler{
		Client:      manager.GetClient(),
		Scheme:      manager.GetScheme(),
		slackClient: slackClient,
		cacheClient: cacheClient,
	}
}

// +kubebuilder:rbac:groups=slack-notifier.mgufrone.dev,resources=slackchannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=slack-notifier.mgufrone.dev,resources=slackchannels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=slack-notifier.mgufrone.dev,resources=slackchannels/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SlackChannel object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *SlackChannelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Namespace).WithValues("slackchannel", req.Name)
	var (
		podList      v1.PodList
		options      []client.ListOption
		slackChannel slacknotifierv1.SlackChannel
	)
	if err = r.Get(ctx, req.NamespacedName, &slackChannel); err != nil {
		return
	}
	spec := slackChannel.Spec
	if !spec.WatchAllNamespace {
		options = append(options, client.InNamespace(req.Namespace))
	}
	if len(spec.Status) == 0 {
		spec.Status = []slacknotifierv1.Status{
			slacknotifierv1.StatusCrashLoopBackoff,
			slacknotifierv1.StatusImagePullBackOff,
			slacknotifierv1.StatusOOMKilled,
			slacknotifierv1.StatusFailedMount,
			slacknotifierv1.StatusContainerConfigError,
			slacknotifierv1.StatusRunContainerError,
		}
	}
	logger.V(4).Info("checking pods")
	if err = r.List(ctx, &podList, options...); err != nil {
		logger.Error(err, "unable to fetch pods")
		return
	}
	statusMap := map[string]slacknotifierv1.Status{
		"CrashLoopBackoff":           slacknotifierv1.StatusCrashLoopBackoff,
		"ImagePullBackOff":           slacknotifierv1.StatusImagePullBackOff,
		"CreateContainerConfigError": slacknotifierv1.StatusContainerConfigError,
		"RunContainerError":          slacknotifierv1.StatusRunContainerError,
	}
	for _, pod := range podList.Items {
		var owner *v1.ObjectReference
		podLog := logger.WithValues("pod", pod.GetName()).WithValues("status", pod.Status.Phase)
		// Get the owner references of the pod
		ownerRefs := pod.GetOwnerReferences()
		for _, ownerRef := range ownerRefs {
			if ownerRef.Controller != nil && *ownerRef.Controller {
				owner = &v1.ObjectReference{
					Kind:       ownerRef.Kind,
					Name:       ownerRef.Name,
					Namespace:  pod.Namespace,
					APIVersion: ownerRef.APIVersion,
				}
				if owner.Kind == "ReplicaSet" {
					replicaSet := &v2.ReplicaSet{}
					if err = r.Get(ctx, client.ObjectKey{Namespace: owner.Namespace, Name: owner.Name}, replicaSet); err != nil {
						podLog.Error(err, "failed to fetch ReplicaSet owner")
					}
					// Get the actual owner (Deployment or StatefulSet) from the ReplicaSet
					for _, rsOwnerRef := range replicaSet.OwnerReferences {
						if rsOwnerRef.Controller != nil && *rsOwnerRef.Controller {
							owner = &v1.ObjectReference{
								Kind:       rsOwnerRef.Kind,
								Name:       rsOwnerRef.Name,
								Namespace:  replicaSet.Namespace,
								APIVersion: rsOwnerRef.APIVersion,
							}
							break
						}
					}
				}
				podLog.Info("found pod owner", "ownerKind", owner.Kind, "ownerName", owner.Name)
				break
			}
		}
		podKey := fmt.Sprintf("%s:%s", pod.Namespace, pod.Name)
		hashedKey := fmt.Sprintf("%x", sha256.Sum256([]byte(podKey)))

		var (
			eventList        v1.EventList
			currentPodStatus slacknotifierv1.Status
			currentPodReason string
		)
		//podLog.Info("fetching events")
		eventFilterOptions := []client.ListOption{
			client.MatchingFields{
				"involvedObject.name":      pod.Name,
				"involvedObject.namespace": pod.Namespace,
				"type":                     v1.EventTypeWarning,
			},
		}
		if err = r.List(ctx, &eventList, eventFilterOptions...); err != nil {
			podLog.Error(err, "unable to fetch events for pod")
		}
		//podLog.Info(fmt.Sprintf("event founds: %d", len(eventList.Items)))

		sort.Slice(eventList.Items, func(i, j int) bool {
			return eventList.Items[i].LastTimestamp.Time.Before(eventList.Items[j].LastTimestamp.Time)
		})

		podLog.Info("check pod status")
		podThread, ok := r.cacheClient.Get(hashedKey)
		// retrieve last container statuses. if one of the container is not ready or started, it will mark the pod as failing
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				currentPodReason = cs.State.Waiting.Message
				//fmt.Printf(" - Container: %s is waiting, Reason: %s\n", cs.Name, cs.State.Waiting.Reason)
				// CrashLoopBackOff, ImagePullBackOff
				if cs.State.Waiting.Reason == "ErrImagePull" {
					cs.State.Waiting.Reason = "ImagePullBackOff"
				}

				if v, ok := statusMap[cs.State.Waiting.Reason]; ok {
					currentPodStatus = v
					break
				}
			}

			// OOMKilled
			if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
				currentPodStatus = slacknotifierv1.StatusOOMKilled
				break
			}
		}
		for _, es := range eventList.Items {
			podLog.Info(fmt.Sprintf("reason: %s; type: %s; desc: %s", es.Reason, es.Type, es.Message))
			if es.Reason == "FailedMount" {
				currentPodStatus = slacknotifierv1.StatusFailedMount
				currentPodReason = es.Message
				break
			}
		}

		// if thread exists and status switched out from failures to working, mark the thread as ok, delete the thread from cache, and send last reply as resolved
		//if ok && helpers.PhaseContains([]v1.PodPhase{v1.PodRunning, v1.PodSucceeded}, pod.Status.Phase) {
		//	podThreadMap := podThread.(map[string]string)
		//	ch := podThreadMap["ch"]
		//	ts := podThreadMap["ts"]
		//	r.slackClient.SendMessage(ch,
		//		slack.MsgOptionTS(ts),
		//		slack.MsgOptionText(fmt.Sprintf("Last Status: %s", pod.Status.Phase), false),
		//	)
		//	threadWriter := bytes.NewBuffer(nil)
		//	_ = render.Thread(render.ThreadData{
		//		Pod:   pod,
		//		Owner: owner,
		//	}, threadWriter)
		//	r.slackClient.UpdateMessage(
		//		ch, ts,
		//		slack.MsgOptionText(threadWriter.String(), false),
		//	)
		//	if err := r.slackClient.AddReaction("white_check_mark", slack.NewRefToMessage(ch, ts)); err != nil {
		//		podLog.WithValues("channel", ch).WithValues("ts", ts).Error(err, "failed to add reaction")
		//	}
		//	r.cacheClient.Del(hashedKey)
		//}
		podLog.WithValues("pod_status", currentPodStatus).Info("pod status finalized")
		if !helpers.StatusContains(spec.Status, currentPodStatus) {
			continue
		}
		podLog.Info("pod match the spec. sending notification")
		// check if it's within backoff limit
		var ch, ts string
		writer := bytes.NewBuffer(nil)
		if err = render.Thread(render.ThreadData{
			Pod:    pod,
			Owner:  owner,
			Reason: currentPodReason,
			Status: currentPodStatus,
		}, writer); err != nil {
			podLog.Error(err, "failed to create thread")
			continue
		}
		if !ok || podThread == nil {
			ch, ts, _ = r.slackClient.PostMessage(spec.Channel, slack.MsgOptionText(writer.String(), false))
			r.cacheClient.Set(hashedKey, map[string]string{
				"ch":         ch,
				"ts":         ts,
				"lastReason": currentPodReason,
			}, 0)
		} else {
			podThreadMap := podThread.(map[string]string)
			lastReason := podThreadMap["lastReason"]
			if lastReason == currentPodReason {
				continue
			}
			ch = podThreadMap["ch"]
			ts = podThreadMap["ts"]
			r.slackClient.UpdateMessage(
				ch, ts,
				slack.MsgOptionText(writer.String(), false),
			)
			r.cacheClient.Set(hashedKey, map[string]string{
				"ch":         ch,
				"ts":         ts,
				"lastReason": currentPodReason,
			}, 0)
		}
		r.slackClient.SendMessage(ch, slack.MsgOptionTS(ts), slack.MsgOptionText(fmt.Sprintf("Last Reason: ```%s```", currentPodReason), false))
		//replyWriter := bytes.NewBuffer(nil)
		//if err = render.Reply(render.ReplyData{Pod: pod, Events: eventList.Items}, replyWriter); err != nil {
		//	podLog.WithValues("ts", ts).WithValues("channel", ch).Error(err, "failed to send reply")
		//	continue
		//}
		//r.slackClient.SendMessage(ch,
		//	slack.MsgOptionTS(ts),
		//	slack.MsgOptionText(replyWriter.String(), false),
		//)
	}

	res.RequeueAfter = time.Second * 60
	return
}

func (r *SlackChannelReconciler) setupIndexes(mgr manager.Manager) error {
	indexer := mgr.GetFieldIndexer()

	// Index events by involvedObject.name
	if err := indexer.IndexField(context.TODO(), &v1.Event{}, "involvedObject.name", func(obj client.Object) []string {
		event, ok := obj.(*v1.Event)
		if !ok {
			return nil
		}
		return []string{event.InvolvedObject.Name}
	}); err != nil {
		return err
	}

	// Index events by involvedObject.namespace
	if err := indexer.IndexField(context.TODO(), &v1.Event{}, "involvedObject.namespace", func(obj client.Object) []string {
		event, ok := obj.(*v1.Event)
		if !ok {
			return nil
		}
		return []string{event.InvolvedObject.Namespace}
	}); err != nil {
		return err
	}
	if err := indexer.IndexField(context.TODO(), &v1.Event{}, "type", func(obj client.Object) []string {
		event, ok := obj.(*v1.Event)
		if !ok {
			return nil
		}
		return []string{event.Type}
	}); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SlackChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.setupIndexes(mgr); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&slacknotifierv1.SlackChannel{}).
		Named("slackchannel").
		Complete(r)
}
