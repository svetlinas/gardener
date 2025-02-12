// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shoot

import (
	"context"
	"fmt"
	"strings"

	gardencorev1alpha1 "github.com/gardener/gardener/pkg/apis/core/v1alpha1"
	gardencorev1alpha1helper "github.com/gardener/gardener/pkg/apis/core/v1alpha1/helper"
	gardencoreinformers "github.com/gardener/gardener/pkg/client/core/informers/externalversions"
	gardencorelisters "github.com/gardener/gardener/pkg/client/core/listers/core/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/logger"
	"github.com/gardener/gardener/pkg/scheduler/apis/config"
	"github.com/gardener/gardener/pkg/scheduler/controller/common"
	schedulerutils "github.com/gardener/gardener/pkg/scheduler/utils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
)

// MsgUnschedulable is the Message for the Event on a Shoot that the Scheduler creates in case it cannot schedule the Shoot to any Seed
const MsgUnschedulable = "Failed to schedule shoot"

func (c *SchedulerController) shootAdd(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		logger.Logger.Errorf("Couldn't get key for object %+v: %v", obj, err)
		return
	}

	newShoot, ok := obj.(*gardencorev1alpha1.Shoot)
	if !ok {
		logger.Logger.Errorf("Couldn't convert object into `core.gardener.cloud/v1alpha1.Shoot`: %+v: %v", obj, err)
		return
	}

	// If the Shoot manifest already specifies a desired Seed cluster, we ignore it.
	if newShoot.Spec.SeedName != nil {
		return
	}

	if newShoot.DeletionTimestamp != nil {
		logger.Logger.Infof("Ignoring shoot '%s' because it has been marked for deletion", newShoot.Name)
		c.shootQueue.Forget(key)
		return
	}

	c.shootQueue.Add(key)
}

func (c *SchedulerController) shootUpdate(oldObj, newObj interface{}) {
	c.shootAdd(newObj)
}

func (c *SchedulerController) reconcileShootKey(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	shoot, err := c.shootLister.Shoots(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		logger.Logger.Debugf("[SCHEDULER SHOOT RECONCILE] %s - skipping because Shoot has been deleted", key)
		return nil
	}
	if err != nil {
		logger.Logger.Infof("[SCHEDULER SHOOT RECONCILE] %s - unable to retrieve object from store: %v", key, err)
		return err
	}
	return c.control.ScheduleShoot(ctx, shoot, key)
}

// SchedulerInterface implements the control logic for updating Seeds. It is implemented as an interface to allow
// for extensions that provide different semantics. Currently, there is only one implementation.
type SchedulerInterface interface {
	// ScheduleShoot implements the control logic for Shoot Scheduling (to a Seed).
	// If an implementation returns a non-nil error, the invocation will be retried respecting the RetrySyncPeriod with exponential backoff.
	ScheduleShoot(ctx context.Context, seed *gardencorev1alpha1.Shoot, key string) error
}

// NewDefaultControl returns a new instance of the default implementation SchedulerInterface that
// implements the documented semantics for Scheduling.
func NewDefaultControl(k8sGardenClient kubernetes.Interface, k8sGardenCoreInformers gardencoreinformers.SharedInformerFactory, recorder record.EventRecorder, config *config.SchedulerConfiguration, shootLister gardencorelisters.ShootLister, seedLister gardencorelisters.SeedLister, cloudProfileLister gardencorelisters.CloudProfileLister) SchedulerInterface {
	return &defaultControl{k8sGardenClient, k8sGardenCoreInformers, recorder, config, shootLister, seedLister, cloudProfileLister}
}

type defaultControl struct {
	k8sGardenClient        kubernetes.Interface
	k8sGardenCoreInformers gardencoreinformers.SharedInformerFactory
	recorder               record.EventRecorder
	config                 *config.SchedulerConfiguration
	shootLister            gardencorelisters.ShootLister
	seedLister             gardencorelisters.SeedLister
	cloudProfileLister     gardencorelisters.CloudProfileLister
}

type executeSchedulingRequest = func(context.Context, *gardencorev1alpha1.Shoot) error

func (c *defaultControl) ScheduleShoot(ctx context.Context, obj *gardencorev1alpha1.Shoot, key string) error {
	var (
		shoot           = obj.DeepCopy()
		schedulerLogger = logger.NewFieldLogger(logger.Logger, "scheduler", "shoot").WithField("shoot", shoot.Name)
	)

	schedulerLogger.Infof("[SCHEDULING SHOOT] using %s strategy", c.config.Schedulers.Shoot.Strategy)

	// If no Seed is referenced, we try to determine an adequate one.
	seed, err := determineSeed(shoot, c.seedLister, c.shootLister, c.cloudProfileLister, c.config.Schedulers.Shoot.Strategy)
	if err != nil {
		c.reportFailedScheduling(shoot, err)
		return err
	}

	updateShoot := func(ctx context.Context, shootToUpdate *gardencorev1alpha1.Shoot) error {
		// need retry logic, because the controller-manager is acting on it at the same time: setting Status to Pending until scheduled
		_, err = kutil.TryUpdateCoreShoot(c.k8sGardenClient.GardenCore(), retry.DefaultBackoff, shootToUpdate.ObjectMeta, func(shoot *gardencorev1alpha1.Shoot) (*gardencorev1alpha1.Shoot, error) {
			if shoot.Spec.SeedName != nil {
				alreadyScheduledErr := common.NewAlreadyScheduledError(fmt.Sprintf("shoot has already a seed assigned when trying to schedule the shoot to %s", *shootToUpdate.Spec.SeedName))
				return nil, &alreadyScheduledErr
			}
			shoot.Spec.SeedName = shootToUpdate.Spec.SeedName
			return shoot, nil
		})
		return err
	}

	if err := UpdateShootToBeScheduledOntoSeed(ctx, shoot, seed, updateShoot); err != nil {
		// there was an external change while trying to schedule the shoot. The shoot is already scheduled. Fine, do not raise an error.
		if _, ok := err.(*common.AlreadyScheduledError); ok {
			return nil
		}
		c.reportFailedScheduling(shoot, err)
		return err
	}

	schedulerLogger.Infof("Shoot '%s' (Cloud Profile '%s', Region '%s') successfully scheduled to seed '%s' using SeedDeterminationStrategy '%s'", shoot.Name, shoot.Spec.CloudProfileName, shoot.Spec.Region, seed.Name, c.config.Schedulers.Shoot.Strategy)
	c.reportSuccessfulScheduling(shoot, seed.Name)
	return nil
}

// determineSeed returns an appropriate Seed cluster (or nil).
func determineSeed(shoot *gardencorev1alpha1.Shoot, seedLister gardencorelisters.SeedLister, shootLister gardencorelisters.ShootLister, cloudProfileLister gardencorelisters.CloudProfileLister, strategy config.CandidateDeterminationStrategy) (*gardencorev1alpha1.Seed, error) {
	seedList, err := seedLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	shootList, err := shootLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	cloudProfile, err := cloudProfileLister.Get(shoot.Spec.CloudProfileName)
	if err != nil {
		return nil, err
	}

	return determineBestSeedCandidate(shoot, cloudProfile, shootList, seedList, strategy)
}

func determineBestSeedCandidate(shoot *gardencorev1alpha1.Shoot, cloudProfile *gardencorev1alpha1.CloudProfile, shootList []*gardencorev1alpha1.Shoot, seedList []*gardencorev1alpha1.Seed, strategy config.CandidateDeterminationStrategy) (*gardencorev1alpha1.Seed, error) {
	var candidates []*gardencorev1alpha1.Seed
	switch strategy {
	case config.SameRegion:
		candidates = determineCandidatesWithSameRegionStrategy(seedList, shoot, candidates)
	case config.MinimalDistance:
		candidates = determineCandidatesWithMinimalDistanceStrategy(seedList, shoot, candidates)
	default:
		return nil, fmt.Errorf("unknown seed determination strategy configured. Strategy: '%s' does not exist. Valid strategies are: %v", strategy, config.Strategies)
	}

	if candidates == nil {
		return nil, fmt.Errorf("no matching seed found for Configuration (Cloud Profile '%s', Region '%s', SeedDeterminationStrategy '%s')", shoot.Spec.CloudProfileName, shoot.Spec.Region, strategy)
	}

	selector := &metav1.LabelSelector{}
	if cloudProfile.Spec.SeedSelector != nil {
		selector = cloudProfile.Spec.SeedSelector
	}
	seedSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("label selector conversion failed: %v for seedSelector: %v", *selector, err)
	}

	// Filter out candidates
	old := candidates
	candidates = nil

	for _, seed := range old {
		if !networksAreDisjunct(seed, shoot) {
			continue
		}
		if !seedSelector.Matches(labels.Set(seed.Labels)) {
			continue
		}
		candidates = append(candidates, seed)
	}

	if candidates == nil {
		return nil, fmt.Errorf("found %d possible seed cluster(s), however none have a disjoint network", len(old))
	}

	// Find the best candidate (i.e. the one managing the smallest number of shoots right now).
	var (
		bestCandidate *gardencorev1alpha1.Seed
		min           *int
		seedUsage     = generateSeedUsageMap(shootList)
	)

	for _, seed := range candidates {
		if numberOfManagedShoots := seedUsage[seed.Name]; min == nil || numberOfManagedShoots < *min {
			bestCandidate = seed
			min = &numberOfManagedShoots
		}
	}

	return bestCandidate, nil
}

func determineCandidatesWithSameRegionStrategy(seedList []*gardencorev1alpha1.Seed, shoot *gardencorev1alpha1.Shoot, candidates []*gardencorev1alpha1.Seed) []*gardencorev1alpha1.Seed {
	// Determine all candidate seed clusters matching the shoot's provider and region.
	for _, seed := range seedList {
		if seed.DeletionTimestamp == nil && seed.Spec.Provider.Type == shoot.Spec.Provider.Type && seed.Spec.Provider.Region == shoot.Spec.Region && !gardencorev1alpha1helper.TaintsHave(seed.Spec.Taints, gardencorev1alpha1.SeedTaintInvisible) && verifySeedAvailability(seed) {
			candidates = append(candidates, seed)
		}
	}
	return candidates
}

func determineCandidatesWithMinimalDistanceStrategy(seeds []*gardencorev1alpha1.Seed, shoot *gardencorev1alpha1.Shoot, candidates []*gardencorev1alpha1.Seed) []*gardencorev1alpha1.Seed {
	if candidates = determineCandidatesWithSameRegionStrategy(seeds, shoot, candidates); candidates != nil {
		return candidates
	}

	var (
		currentMaxMatchingCharacters int
		shootRegion                  = shoot.Spec.Region
	)

	// Determine all candidate seed clusters with matching cloud provider but different region that are lexicographically closest to the shoot
	for _, seed := range seeds {
		if seed.DeletionTimestamp == nil && seed.Spec.Provider.Type == shoot.Spec.Provider.Type && !gardencorev1alpha1helper.TaintsHave(seed.Spec.Taints, gardencorev1alpha1.SeedTaintInvisible) && verifySeedAvailability(seed) {
			seedRegion := seed.Spec.Provider.Region

			for currentMaxMatchingCharacters < len(shootRegion) {
				if strings.HasPrefix(seedRegion, shootRegion[:currentMaxMatchingCharacters+1]) {
					candidates = []*gardencorev1alpha1.Seed{}
					currentMaxMatchingCharacters++
					continue
				} else if strings.HasPrefix(seedRegion, shootRegion[:currentMaxMatchingCharacters]) {
					candidates = append(candidates, seed)
				}
				break
			}
		}
	}
	return candidates
}

func generateSeedUsageMap(shootList []*gardencorev1alpha1.Shoot) map[string]int {
	m := map[string]int{}

	for _, shoot := range shootList {
		if seed := shoot.Spec.SeedName; seed != nil {
			m[*seed]++
		}
	}

	return m
}

func networksAreDisjunct(seed *gardencorev1alpha1.Seed, shoot *gardencorev1alpha1.Shoot) bool {
	return len(schedulerutils.ValidateNetworkDisjointedness(seed.Spec.Networks, shoot.Spec.Networking.Nodes, shoot.Spec.Networking.Pods, shoot.Spec.Networking.Services, field.NewPath(""))) == 0
}

func verifySeedAvailability(seed *gardencorev1alpha1.Seed) bool {
	if cond := gardencorev1alpha1helper.GetCondition(seed.Status.Conditions, gardencorev1alpha1.SeedAvailable); cond != nil {
		return cond.Status == gardencorev1alpha1.ConditionTrue
	}
	return false
}

// UpdateShootToBeScheduledOntoSeed sets the seed name where the shoot should be scheduled on. Then it executes the actual update call to the API server. The call is capsuled to allow for easier testing.
func UpdateShootToBeScheduledOntoSeed(ctx context.Context, shoot *gardencorev1alpha1.Shoot, seed *gardencorev1alpha1.Seed, executeSchedulingRequest executeSchedulingRequest) error {
	shoot.Spec.SeedName = &seed.Name
	return executeSchedulingRequest(ctx, shoot)
}

func (c *defaultControl) reportFailedScheduling(shoot *gardencorev1alpha1.Shoot, err error) {
	c.reportEvent(shoot, corev1.EventTypeWarning, gardencorev1alpha1.ShootEventSchedulingFailed, MsgUnschedulable+" '%s' : %+v", shoot.Name, err)
}

func (c *defaultControl) reportSuccessfulScheduling(shoot *gardencorev1alpha1.Shoot, seedName string) {
	c.reportEvent(shoot, corev1.EventTypeNormal, gardencorev1alpha1.ShootEventSchedulingSuccessful, "Scheduled to seed '%s'", seedName)
}

func (c *defaultControl) reportEvent(project *gardencorev1alpha1.Shoot, eventType string, eventReason, messageFmt string, args ...interface{}) {
	c.recorder.Eventf(project, eventType, eventReason, messageFmt, args...)
}
