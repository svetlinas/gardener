// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package validator_test

import (
	"fmt"
	"time"

	"github.com/gardener/gardener/pkg/apis/garden"
	gardeninformers "github.com/gardener/gardener/pkg/client/garden/informers/internalversion"
	. "github.com/gardener/gardener/plugin/pkg/shoot/validator"
	"github.com/gardener/gardener/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/admission"
)

var _ = Describe("validator", func() {
	Describe("#Admit", func() {
		var (
			admissionHandler      *ValidateShoot
			gardenInformerFactory gardeninformers.SharedInformerFactory
			cloudProfile          garden.CloudProfile
			seed                  garden.Seed
			project               garden.Project
			shoot                 garden.Shoot

			podCIDR     = "100.96.0.0/11"
			serviceCIDR = "100.64.0.0/13"
			nodesCIDR   = "10.250.0.0/16"
			k8sNetworks = garden.K8SNetworks{
				Pods:     &podCIDR,
				Services: &serviceCIDR,
				Nodes:    &nodesCIDR,
			}

			falseVar = false

			seedName      = "seed"
			namespaceName = "garden-my-project"
			projectName   = "my-project"

			unmanagedDNSProvider = garden.DNSUnmanaged
			baseDomain           = "example.com"

			validMachineImageName         = "some-machineimage"
			validMachineImageVersions     = []garden.ExpirableVersion{{Version: "0.0.1"}}
			validShootMachineImageVersion = "0.0.1"

			seedPodsCIDR     = "10.241.128.0/17"
			seedServicesCIDR = "10.241.0.0/17"
			seedNodesCIDR    = "10.240.0.0/16"

			projectBase = garden.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
				},
				Spec: garden.ProjectSpec{
					Namespace: &namespaceName,
				},
			}
			cloudProfileBase = garden.CloudProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "profile",
				},
				Spec: garden.CloudProfileSpec{
					Type: "unknown",
					Kubernetes: garden.KubernetesSettings{
						Versions: []garden.ExpirableVersion{{Version: "1.6.4"}},
					},
					MachineImages: []garden.CloudProfileMachineImage{
						{
							Name:     validMachineImageName,
							Versions: validMachineImageVersions,
						},
					},
					MachineTypes: []garden.MachineType{
						{
							Name:   "machine-type-1",
							CPU:    resource.MustParse("2"),
							GPU:    resource.MustParse("0"),
							Memory: resource.MustParse("100Gi"),
						},
						{
							Name:   "machine-type-old",
							CPU:    resource.MustParse("2"),
							GPU:    resource.MustParse("0"),
							Memory: resource.MustParse("100Gi"),
							Usable: &falseVar,
						},
					},
					VolumeTypes: []garden.VolumeType{
						{
							Name:  "volume-type-1",
							Class: "super-premium",
						},
					},
					Regions: []garden.Region{
						{
							Name:  "europe",
							Zones: []garden.AvailabilityZone{{Name: "europe-a"}},
						},
						{
							Name:  "asia",
							Zones: []garden.AvailabilityZone{{Name: "asia-a"}},
						},
					},
				},
			}
			seedBase = garden.Seed{
				ObjectMeta: metav1.ObjectMeta{
					Name: seedName,
				},
				Spec: garden.SeedSpec{
					Networks: garden.SeedNetworks{
						Pods:     seedPodsCIDR,
						Services: seedServicesCIDR,
						Nodes:    seedNodesCIDR,
					},
				},
			}
			shootBase = garden.Shoot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shoot",
					Namespace: namespaceName,
				},
				Spec: garden.ShootSpec{
					CloudProfileName:  "profile",
					Region:            "europe",
					SecretBindingName: "my-secret",
					SeedName:          &seedName,
					DNS: &garden.DNS{
						Domain: test.MakeStrPointer(fmt.Sprintf("shoot.%s", baseDomain)),
						Providers: []garden.DNSProvider{
							{
								Type: &unmanagedDNSProvider,
							},
						},
					},
					Kubernetes: garden.Kubernetes{
						Version: "1.6.4",
					},
					Networking: garden.Networking{
						Nodes:    *k8sNetworks.Nodes,
						Pods:     k8sNetworks.Pods,
						Services: k8sNetworks.Services,
					},
					Provider: garden.Provider{
						Type: "unknown",
						Workers: []garden.Worker{
							{
								Name: "worker-name",
								Machine: garden.Machine{
									Type: "machine-type-1",
								},
								Minimum: 1,
								Maximum: 1,
								Volume: &garden.Volume{
									Size: "40Gi",
									Type: "volume-type-1",
								},
								Zones: []string{"europe-a"},
							},
						},
					},
				},
			}
		)

		BeforeEach(func() {
			project = projectBase
			cloudProfile = *cloudProfileBase.DeepCopy()
			seed = seedBase
			shoot = *shootBase.DeepCopy()

			admissionHandler, _ = New()
			admissionHandler.AssignReadyFunc(func() bool { return true })
			gardenInformerFactory = gardeninformers.NewSharedInformerFactory(nil, 0)
			admissionHandler.SetInternalGardenInformerFactory(gardenInformerFactory)
		})

		AfterEach(func() {
			cloudProfile.Spec.AWS = nil
			cloudProfile.Spec.Azure = nil
			cloudProfile.Spec.GCP = nil
			cloudProfile.Spec.Packet = nil
			cloudProfile.Spec.OpenStack = nil

			shoot.Spec.Cloud.AWS = nil
			shoot.Spec.Cloud.Azure = nil
			shoot.Spec.Cloud.GCP = nil
			shoot.Spec.Cloud.Packet = nil
			shoot.Spec.Cloud.OpenStack = nil
			shoot.Spec.Kubernetes = garden.Kubernetes{
				KubeControllerManager: nil,
			}
		})

		// The verification of protection is independent of the Cloud Provider (being checked before). We use AWS.
		Context("VALIDATION: Shoot references a Seed already -  validate user provided seed regarding protection", func() {
			var (
				oldShoot *garden.Shoot
			)

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()

				// set seed name
				shoot.Spec.SeedName = &seedName

				// set old shoot for update
				oldShoot = shoot.DeepCopy()
				oldShoot.Spec.SeedName = nil
			})

			It("create should pass because the Seed specified in shoot manifest is not protected and shoot is not in garden namespace", func() {
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("update should pass because the Seed specified in shoot manifest is not protected and shoot is not in garden namespace", func() {
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, oldShoot, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Update, false, nil)

				err := admissionHandler.Admit(attrs, nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("create should pass because shoot is not in garden namespace and seed is not protected", func() {
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("update should pass because shoot is not in garden namespace and seed is not protected", func() {
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, oldShoot, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("create should fail because shoot is not in garden namespace and seed is protected", func() {
				seed.Spec.Taints = []garden.SeedTaint{{Key: garden.SeedTaintProtected}}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("update should fail because shoot is not in garden namespace and seed is protected", func() {
				seed.Spec.Taints = []garden.SeedTaint{{Key: garden.SeedTaintProtected}}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, oldShoot, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Update, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("create should pass because shoot is in garden namespace and seed is protected", func() {
				ns := "garden"
				shoot.Namespace = ns
				project.Spec.Namespace = &ns
				seed.Spec.Taints = []garden.SeedTaint{{Key: garden.SeedTaintProtected}}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

			It("update should pass because shoot is in garden namespace and seed is protected", func() {
				ns := "garden"
				shoot.Namespace = ns
				project.Spec.Namespace = &ns
				seed.Spec.Taints = []garden.SeedTaint{{Key: garden.SeedTaintProtected}}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, oldShoot, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Update, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

			It("create should pass because shoot is in garden namespace and seed is not protected", func() {
				ns := "garden"
				shoot.Namespace = ns
				project.Spec.Namespace = &ns

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

			It("update should pass because shoot is in garden namespace and seed is not protected", func() {
				ns := "garden"
				shoot.Namespace = ns
				project.Spec.Namespace = &ns

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, oldShoot, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Update, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

		})

		Context("name/project length checks", func() {
			It("should reject Shoot resources with two consecutive hyphens in project name", func() {
				twoConsecutiveHyphensName := "n--o"
				project.ObjectMeta = metav1.ObjectMeta{
					Name: twoConsecutiveHyphensName,
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsBadRequest(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("consecutive hyphens"))
			})

			It("should reject create operations on Shoot resources in projects which shall be deleted", func() {
				deletionTimestamp := metav1.NewTime(time.Now())
				project.ObjectMeta.DeletionTimestamp = &deletionTimestamp

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("already marked for deletion"))
			})

			It("should reject Shoot resources with not fulfilling the length constraints", func() {
				tooLongName := "too-long-namespace"
				project.ObjectMeta = metav1.ObjectMeta{
					Name: tooLongName,
				}
				shoot.ObjectMeta = metav1.ObjectMeta{
					Name:      "too-long-name",
					Namespace: namespaceName,
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsBadRequest(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("name must not exceed"))
			})

			It("should not testing length constraints for operations other than CREATE", func() {
				shortName := "short"
				projectName := "too-long-long-long-label"
				project.ObjectMeta = metav1.ObjectMeta{
					Name: projectName,
				}
				shoot.ObjectMeta = metav1.ObjectMeta{
					Name:      shortName,
					Namespace: shortName,
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Update, false, nil)
				err := admissionHandler.Admit(attrs, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).NotTo(ContainSubstring("name must not exceed"))

				attrs = admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Delete, false, nil)
				err = admissionHandler.Admit(attrs, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).NotTo(ContainSubstring("name must not exceed"))
			})
		})

		It("should reject because the referenced cloud profile was not found", func() {
			attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

			err := admissionHandler.Admit(attrs, nil)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		It("should reject because the referenced seed was not found", func() {
			gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
			gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
			attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

			err := admissionHandler.Admit(attrs, nil)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		It("should reject because the referenced project was not found", func() {
			gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
			gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
			attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

			err := admissionHandler.Admit(attrs, nil)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		It("should reject because the cloud provider in shoot and profile differ", func() {
			cloudProfile.Spec.Type = "gcp"
			shoot.Spec.Provider.Type = "aws"

			gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
			gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
			gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
			attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

			err := admissionHandler.Admit(attrs, nil)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		Context("tests for AWS cloud", func() {
			var (
				awsProfile = &garden.AWSProfile{
					Constraints: garden.AWSConstraints{
						DNSProviders: []garden.DNSProviderConstraint{
							{
								Name: garden.DNSUnmanaged,
							},
						},
					},
				}
				workers = []garden.Worker{
					{
						Name: "worker-name",
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Minimum: 1,
						Maximum: 1,
						Volume: &garden.Volume{
							Size: "10Gi",
							Type: "volume-type-1",
						},
					},
				}
				zones        = []string{"europe-a"}
				machineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: validShootMachineImageVersion,
				}
				awsCloud = &garden.AWSCloud{}
			)

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()
				awsCloud.Networks = garden.AWSNetworks{K8SNetworks: k8sNetworks}
				awsCloud.Workers = workers
				awsCloud.Zones = zones
				awsCloud.MachineImage = machineImage
				cloudProfile.Spec.Type = "aws"
				cloudProfile.Spec.AWS = awsProfile
				shoot.Spec.Cloud.AWS = awsCloud
				shoot.Spec.Provider.Type = "aws"
			})

			It("should pass because no seed has to be specified (however can be). The scheduler sets the seed instead.", func() {
				shoot.Spec.SeedName = nil
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject because the shoot node and the seed node networks intersect", func() {
				shoot.Spec.Cloud.AWS.Networks.Nodes = &seedNodesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot pod and the seed pod networks intersect", func() {
				shoot.Spec.Cloud.AWS.Networks.Pods = &seedPodsCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot service and the seed service networks intersect", func() {
				shoot.Spec.Cloud.AWS.Networks.Services = &seedServicesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is already used by another shoot", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is a subdomain of a domain already used by another shoot", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				subdomain := fmt.Sprintf("subdomain.%s", *anotherShoot.Spec.DNS.Domain)
				shoot.Spec.DNS.Domain = &subdomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is a subdomain of a domain already used by another shoot (case one)", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				subdomain := fmt.Sprintf("subdomain.%s", *anotherShoot.Spec.DNS.Domain)
				shoot.Spec.DNS.Domain = &subdomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is a subdomain of a domain already used by another shoot (case two)", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				shoot.Spec.DNS.Domain = &baseDomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should allow because the specified domain is not a subdomain of a domain already used by another shoot", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				anotherDomain := fmt.Sprintf("someprefix%s", *anotherShoot.Spec.DNS.Domain)
				shoot.Spec.DNS.Domain = &anotherDomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(BeNil())
			})

			It("should reject due to an invalid kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.2.3"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should default a major.minor kubernetes version to latest patch version", func() {
				shoot.Spec.Kubernetes.Version = "1.6"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.6.6"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, highestPatchVersion, garden.ExpirableVersion{Version: "1.7.1"}, garden.ExpirableVersion{Version: "1.7.2"})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Not(HaveOccurred()))
				Expect(shoot.Spec.Kubernetes.Version).To(Equal(highestPatchVersion.Version))
			})

			It("should reject: default only exactly matching minor kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.8"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.81.5"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, garden.ExpirableVersion{Version: "1.81.0"}, highestPatchVersion)

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine image", func() {
				shoot.Spec.Cloud.AWS.MachineImage = &garden.ShootMachineImage{
					Name:    "not-supported",
					Version: "not-supported",
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to a machine image with expiration date in the past", func() {
				imageVersionExpired := "0.0.1-beta"

				shoot.Spec.Cloud.AWS.MachineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: imageVersionExpired,
				}

				timeInThePast := metav1.Now().Add(time.Second * -1000)
				cloudProfile.Spec.MachineImages = append(cloudProfile.Spec.MachineImages, garden.CloudProfileMachineImage{
					Name: validMachineImageName,
					Versions: []garden.ExpirableVersion{
						{
							Version:        imageVersionExpired,
							ExpirationDate: &metav1.Time{Time: timeInThePast},
						},
					},
				}, garden.CloudProfileMachineImage{
					Name: "other-image-name",
					Versions: []garden.ExpirableVersion{
						{
							Version: imageVersionExpired,
						},
					},
				})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should not reject due to an usable machine type", func() {
				shoot.Spec.Cloud.AWS.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject due to a not usable machine type", func() {
				shoot.Spec.Cloud.AWS.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-old",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine type", func() {
				shoot.Spec.Cloud.AWS.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid volume type", func() {
				shoot.Spec.Cloud.AWS.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Volume: &garden.Volume{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid zone", func() {
				shoot.Spec.Cloud.AWS.Zones = []string{"invalid-zone"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})

		Context("tests for Azure cloud", func() {
			var (
				azureProfile = &garden.AzureProfile{
					Constraints: garden.AzureConstraints{
						DNSProviders: []garden.DNSProviderConstraint{
							{
								Name: garden.DNSUnmanaged,
							},
						},
					},
					CountFaultDomains: []garden.AzureDomainCount{
						{
							Region: "europe",
							Count:  1,
						},
						{
							Region: "australia",
							Count:  1,
						},
					},
					CountUpdateDomains: []garden.AzureDomainCount{
						{
							Region: "europe",
							Count:  1,
						},
						{
							Region: "asia",
							Count:  1,
						},
					},
				}
				workers = []garden.Worker{
					{
						Name: "worker-name",
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Minimum: 1,
						Maximum: 1,
						Volume: &garden.Volume{
							Size: "10Gi",
							Type: "volume-type-1",
						},
					},
				}
				machineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: validShootMachineImageVersion,
				}
				azureCloud = &garden.AzureCloud{}
			)

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()
				cloudProfile.Spec.Azure = azureProfile
				azureCloud.Networks = garden.AzureNetworks{K8SNetworks: k8sNetworks}
				azureCloud.Workers = workers
				azureCloud.MachineImage = machineImage
				cloudProfile.Spec.Type = "azure"
				shoot.Spec.Provider.Type = "azure"
				shoot.Spec.Cloud.Azure = azureCloud
			})

			It("should pass because no seed has to be specified (however can be). The scheduler sets the seed instead.", func() {
				shoot.Spec.SeedName = nil
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject because the shoot node and the seed node networks intersect", func() {
				shoot.Spec.Cloud.Azure.Networks.Nodes = &seedNodesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot pod and the seed pod networks intersect", func() {
				shoot.Spec.Cloud.Azure.Networks.Pods = &seedPodsCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot service and the seed service networks intersect", func() {
				shoot.Spec.Cloud.Azure.Networks.Services = &seedServicesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.2.3"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should default a major.minor kubernetes version to latest patch version", func() {
				shoot.Spec.Kubernetes.Version = "1.6"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.6.6"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, highestPatchVersion, garden.ExpirableVersion{Version: "1.7.1"}, garden.ExpirableVersion{Version: "1.7.2"})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Not(HaveOccurred()))
				Expect(shoot.Spec.Kubernetes.Version).To(Equal(highestPatchVersion.Version))
			})

			It("should reject: default only exactly matching minor kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.8"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.81.5"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, garden.ExpirableVersion{Version: "1.81.0"}, highestPatchVersion)

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It(" ", func() {
				shoot.Spec.Cloud.Azure.MachineImage = &garden.ShootMachineImage{
					Name:    "not-supported",
					Version: "not-supported",
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to a machine image with expiration date in the past", func() {
				imageVersionExpired := "0.0.1-beta"

				shoot.Spec.Cloud.Azure.MachineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: imageVersionExpired,
				}

				timeInThePast := metav1.Now().Add(time.Second * -1000)
				cloudProfile.Spec.MachineImages = append(cloudProfile.Spec.MachineImages, garden.CloudProfileMachineImage{
					Name: validMachineImageName,
					Versions: []garden.ExpirableVersion{
						{
							Version:        imageVersionExpired,
							ExpirationDate: &metav1.Time{Time: timeInThePast},
						},
					},
				}, garden.CloudProfileMachineImage{
					Name: "other-image-name",
					Versions: []garden.ExpirableVersion{
						{
							Version: imageVersionExpired,
						},
					},
				})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine type", func() {
				shoot.Spec.Cloud.Azure.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid volume type", func() {
				shoot.Spec.Cloud.Azure.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Volume: &garden.Volume{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid region where no fault domain count has been specified", func() {
				shoot.Spec.Region = "asia"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid region where no update domain count has been specified", func() {
				shoot.Spec.Region = "australia"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})

		Context("tests for GCP cloud", func() {
			var (
				gcpProfile = &garden.GCPProfile{
					Constraints: garden.GCPConstraints{
						DNSProviders: []garden.DNSProviderConstraint{
							{
								Name: garden.DNSUnmanaged,
							},
						},
					},
				}
				workers = []garden.Worker{
					{
						Name: "worker-name",
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Minimum: 1,
						Maximum: 1,
						Volume: &garden.Volume{
							Size: "10Gi",
							Type: "volume-type-1",
						},
					},
				}
				zones        = []string{"europe-a"}
				machineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: validShootMachineImageVersion,
				}
				gcpCloud = &garden.GCPCloud{}
			)

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()
				gcpCloud.Networks = garden.GCPNetworks{K8SNetworks: k8sNetworks}
				gcpCloud.Workers = workers
				gcpCloud.Zones = zones
				gcpCloud.MachineImage = machineImage
				cloudProfile.Spec.Type = "gcp"
				shoot.Spec.Provider.Type = "gcp"
				cloudProfile.Spec.GCP = gcpProfile
				shoot.Spec.Cloud.GCP = gcpCloud
			})

			It("should pass because no seed has to be specified (however can be). The scheduler sets the seed instead.", func() {
				shoot.Spec.SeedName = nil
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject because the shoot node and the seed node networks intersect", func() {
				shoot.Spec.Cloud.GCP.Networks.Nodes = &seedNodesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot pod and the seed pod networks intersect", func() {
				shoot.Spec.Cloud.GCP.Networks.Pods = &seedPodsCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot service and the seed service networks intersect", func() {
				shoot.Spec.Cloud.GCP.Networks.Services = &seedServicesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.2.3"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should default a major.minor kubernetes version to latest patch version", func() {
				shoot.Spec.Kubernetes.Version = "1.6"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.6.6"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, highestPatchVersion, garden.ExpirableVersion{Version: "1.7.1"}, garden.ExpirableVersion{Version: "1.7.2"})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Not(HaveOccurred()))
				Expect(shoot.Spec.Kubernetes.Version).To(Equal(highestPatchVersion.Version))
			})

			It("should reject: default only exactly matching minor kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.8"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.81.5"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, garden.ExpirableVersion{Version: "1.81.0"}, highestPatchVersion)

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine image", func() {
				shoot.Spec.Cloud.GCP.MachineImage = &garden.ShootMachineImage{
					Name:    "not-supported",
					Version: "not-supported",
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to a machine image with expiration date in the past", func() {
				imageVersionExpired := "0.0.1-beta"

				shoot.Spec.Cloud.GCP.MachineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: imageVersionExpired,
				}

				timeInThePast := metav1.Now().Add(time.Second * -1000)
				cloudProfile.Spec.MachineImages = append(cloudProfile.Spec.MachineImages, garden.CloudProfileMachineImage{
					Name: validMachineImageName,
					Versions: []garden.ExpirableVersion{
						{
							Version:        imageVersionExpired,
							ExpirationDate: &metav1.Time{Time: timeInThePast},
						},
					},
				}, garden.CloudProfileMachineImage{
					Name: "other-image-name",
					Versions: []garden.ExpirableVersion{
						{
							Version: imageVersionExpired,
						},
					},
				})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine type", func() {
				shoot.Spec.Cloud.GCP.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid volume type", func() {
				shoot.Spec.Cloud.GCP.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Volume: &garden.Volume{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid zone", func() {
				shoot.Spec.Cloud.GCP.Zones = []string{"invalid-zone"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})

		Context("tests for Packet cloud", func() {
			var (
				packetProfile = &garden.PacketProfile{
					Constraints: garden.PacketConstraints{
						DNSProviders: []garden.DNSProviderConstraint{
							{
								Name: garden.DNSUnmanaged,
							},
						},
					},
				}
				workers = []garden.Worker{
					{
						Name: "worker-name",
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Minimum: 1,
						Maximum: 1,
						Volume: &garden.Volume{
							Size: "10Gi",
							Type: "volume-type-1",
						},
					},
				}
				zones        = []string{"europe-a"}
				machineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: validShootMachineImageVersion,
				}
				packetCloud = &garden.PacketCloud{}
			)

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()
				packetCloud.Networks = garden.PacketNetworks{K8SNetworks: k8sNetworks}
				packetCloud.Workers = workers
				packetCloud.Zones = zones
				packetCloud.MachineImage = machineImage
				cloudProfile.Spec.Type = "packet"
				shoot.Spec.Provider.Type = "packet"
				cloudProfile.Spec.Packet = packetProfile
				shoot.Spec.Cloud.Packet = packetCloud
			})

			It("should pass because no seed has to be specified (however can be). The scheduler sets the seed instead.", func() {
				shoot.Spec.SeedName = nil
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject because the shoot pod and the seed pod networks intersect", func() {
				shoot.Spec.Cloud.Packet.Networks.Pods = &seedPodsCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot service and the seed service networks intersect", func() {
				shoot.Spec.Cloud.Packet.Networks.Services = &seedServicesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.2.3"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should default a major.minor kubernetes version to latest patch version", func() {
				shoot.Spec.Kubernetes.Version = "1.6"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.6.6"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, highestPatchVersion, garden.ExpirableVersion{Version: "1.7.1"}, garden.ExpirableVersion{Version: "1.7.2"})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Not(HaveOccurred()))
				Expect(shoot.Spec.Kubernetes.Version).To(Equal(highestPatchVersion.Version))
			})

			It("should reject: default only exactly matching minor kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.8"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.81.5"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, garden.ExpirableVersion{Version: "1.81.0"}, highestPatchVersion)

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine image", func() {
				shoot.Spec.Cloud.Packet.MachineImage = &garden.ShootMachineImage{
					Name:    "not-supported",
					Version: "not-supported",
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to a machine image with expiration date in the past", func() {
				imageVersionExpired := "0.0.1-beta"

				shoot.Spec.Cloud.Packet.MachineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: imageVersionExpired,
				}

				timeInThePast := metav1.Now().Add(time.Second * -1000)
				cloudProfile.Spec.MachineImages = append(cloudProfile.Spec.MachineImages, garden.CloudProfileMachineImage{
					Name: validMachineImageName,
					Versions: []garden.ExpirableVersion{
						{
							Version:        imageVersionExpired,
							ExpirationDate: &metav1.Time{Time: timeInThePast},
						},
					},
				}, garden.CloudProfileMachineImage{
					Name: "other-image-name",
					Versions: []garden.ExpirableVersion{
						{
							Version: imageVersionExpired,
						},
					},
				})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine type", func() {
				shoot.Spec.Cloud.Packet.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid volume type", func() {
				shoot.Spec.Cloud.Packet.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Volume: &garden.Volume{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid zone", func() {
				shoot.Spec.Cloud.Packet.Zones = []string{"invalid-zone"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})

		Context("tests for OpenStack cloud", func() {
			var (
				openStackProfile = &garden.OpenStackProfile{
					Constraints: garden.OpenStackConstraints{
						DNSProviders: []garden.DNSProviderConstraint{
							{
								Name: garden.DNSUnmanaged,
							},
						},
						FloatingPools: []garden.OpenStackFloatingPool{
							{
								Name: "pool",
							},
						},
						LoadBalancerProviders: []garden.OpenStackLoadBalancerProvider{
							{
								Name: "haproxy",
							},
						},
					},
				}
				workers = []garden.Worker{
					{
						Name: "worker-name",
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Minimum: 1,
						Maximum: 1,
					},
				}
				zones        = []string{"europe-a"}
				machineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: validShootMachineImageVersion,
				}
				openStackCloud = &garden.OpenStackCloud{}
			)

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()
				openStackCloud.FloatingPoolName = "pool"
				openStackCloud.LoadBalancerProvider = "haproxy"
				openStackCloud.Networks = garden.OpenStackNetworks{K8SNetworks: k8sNetworks}
				openStackCloud.Workers = workers
				openStackCloud.Zones = zones
				openStackCloud.MachineImage = machineImage
				cloudProfile.Spec.Type = "openstack"
				shoot.Spec.Provider.Type = "openstack"
				cloudProfile.Spec.OpenStack = openStackProfile
				shoot.Spec.Cloud.OpenStack = openStackCloud
			})

			It("should pass because no seed has to be specified (however can be). The scheduler sets the seed instead.", func() {
				shoot.Spec.SeedName = nil
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject because the shoot node and the seed node networks intersect", func() {
				shoot.Spec.Cloud.OpenStack.Networks.Nodes = &seedNodesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot pod and the seed pod networks intersect", func() {
				shoot.Spec.Cloud.OpenStack.Networks.Pods = &seedPodsCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot service and the seed service networks intersect", func() {
				shoot.Spec.Cloud.OpenStack.Networks.Services = &seedServicesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should not reject due to an undefined dns domain", func() {
				shoot.Spec.DNS.Domain = nil

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Succeed())
			})

			It("should reject due to an invalid floating pool name", func() {
				shoot.Spec.Cloud.OpenStack.FloatingPoolName = "invalid-pool"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.2.3"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should default a major.minor kubernetes version to latest patch version", func() {
				shoot.Spec.Kubernetes.Version = "1.6"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.6.6"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, highestPatchVersion, garden.ExpirableVersion{Version: "1.7.1"}, garden.ExpirableVersion{Version: "1.7.2"})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Not(HaveOccurred()))
				Expect(shoot.Spec.Kubernetes.Version).To(Equal(highestPatchVersion.Version))
			})

			It("should reject: default only exactly matching minor kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.8"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.81.5"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, garden.ExpirableVersion{Version: "1.81.0"}, highestPatchVersion)

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid load balancer provider", func() {
				shoot.Spec.Cloud.OpenStack.LoadBalancerProvider = "invalid-provider"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine image", func() {
				shoot.Spec.Cloud.OpenStack.MachineImage = &garden.ShootMachineImage{
					Name:    "not-supported",
					Version: "not-supported",
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to a machine image with expiration date in the past", func() {
				imageVersionExpired := "0.0.1-beta"

				shoot.Spec.Cloud.OpenStack.MachineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: imageVersionExpired,
				}

				timeInThePast := metav1.Now().Add(time.Second * -1000)
				cloudProfile.Spec.MachineImages = append(cloudProfile.Spec.MachineImages, garden.CloudProfileMachineImage{
					Name: validMachineImageName,
					Versions: []garden.ExpirableVersion{
						{
							Version:        imageVersionExpired,
							ExpirationDate: &metav1.Time{Time: timeInThePast},
						},
					},
				}, garden.CloudProfileMachineImage{
					Name: "other-image-name",
					Versions: []garden.ExpirableVersion{
						{
							Version: imageVersionExpired,
						},
					},
				})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine type", func() {
				shoot.Spec.Cloud.OpenStack.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid zone", func() {
				shoot.Spec.Cloud.OpenStack.Zones = []string{"invalid-zone"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})

		Context("tests for Alicloud", func() {
			var (
				alicloudProfile = &garden.AlicloudProfile{
					Constraints: garden.AlicloudConstraints{
						DNSProviders: []garden.DNSProviderConstraint{
							{
								Name: garden.DNSUnmanaged,
							},
						},
					},
				}
				workers = []garden.Worker{
					{
						Name: "worker-name",
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Minimum: 1,
						Maximum: 1,
						Volume: &garden.Volume{
							Size: "10Gi",
							Type: "volume-type-1",
						},
					},
				}
				zones        = []string{"europe-a"}
				machineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: validShootMachineImageVersion,
				}
				aliCloud = &garden.Alicloud{}
			)

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()
				aliCloud.Networks = garden.AlicloudNetworks{K8SNetworks: k8sNetworks}
				aliCloud.Workers = workers
				aliCloud.Zones = zones
				aliCloud.MachineImage = machineImage
				cloudProfile.Spec.Type = "alicloud"
				shoot.Spec.Provider.Type = "alicloud"
				cloudProfile.Spec.Alicloud = alicloudProfile
				shoot.Spec.Cloud.Alicloud = aliCloud
			})

			It("should pass because no seed has to be specified (however can be). The scheduler sets the seed instead.", func() {
				shoot.Spec.SeedName = nil
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject because the shoot node and the seed node networks intersect", func() {
				shoot.Spec.Cloud.Alicloud.Networks.Nodes = &seedNodesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot pod and the seed pod networks intersect", func() {
				shoot.Spec.Cloud.Alicloud.Networks.Pods = &seedPodsCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot service and the seed service networks intersect", func() {
				shoot.Spec.Cloud.Alicloud.Networks.Services = &seedServicesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.2.3"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should default a major.minor kubernetes version to latest patch version", func() {
				shoot.Spec.Kubernetes.Version = "1.6"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.6.6"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, highestPatchVersion, garden.ExpirableVersion{Version: "1.7.1"}, garden.ExpirableVersion{Version: "1.7.2"})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Not(HaveOccurred()))
				Expect(shoot.Spec.Kubernetes.Version).To(Equal(highestPatchVersion.Version))
			})

			It("should reject: default only exactly matching minor kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.8"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.81.5"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, garden.ExpirableVersion{Version: "1.81.0"}, highestPatchVersion)

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine image", func() {
				shoot.Spec.Cloud.Alicloud.MachineImage = &garden.ShootMachineImage{
					Name:    "not-supported",
					Version: "not-supported",
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to a machine image with expiration date in the past", func() {
				imageVersionExpired := "0.0.1-beta"

				shoot.Spec.Cloud.Alicloud.MachineImage = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: imageVersionExpired,
				}

				timeInThePast := metav1.Now().Add(time.Second * -1000)
				cloudProfile.Spec.MachineImages = append(cloudProfile.Spec.MachineImages, garden.CloudProfileMachineImage{
					Name: validMachineImageName,
					Versions: []garden.ExpirableVersion{
						{
							Version:        imageVersionExpired,
							ExpirationDate: &metav1.Time{Time: timeInThePast},
						},
					},
				}, garden.CloudProfileMachineImage{
					Name: "other-image-name",
					Versions: []garden.ExpirableVersion{
						{
							Version: imageVersionExpired,
						},
					},
				})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine type", func() {
				shoot.Spec.Cloud.Alicloud.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid volume type", func() {
				shoot.Spec.Cloud.Alicloud.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Volume: &garden.Volume{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid zone", func() {
				shoot.Spec.Cloud.Alicloud.Zones = []string{"invalid-zone"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an machine type is not available in shoot zones", func() {
				shoot.Spec.Cloud.Alicloud.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
					},
				}

				cloudProfile.Spec.Regions[0].Zones[0].UnavailableMachineTypes = []string{"machine-type-1"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an volume type is not available in shoot zones", func() {
				shoot.Spec.Cloud.Alicloud.Workers = []garden.Worker{
					{
						Volume: &garden.Volume{
							Type: "volume-type-1",
						},
					},
				}

				cloudProfile.Spec.Regions[0].Zones[0].UnavailableVolumeTypes = []string{"volume-type-1"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})

		Context("tests for unknown provider", func() {
			var workers = []garden.Worker{
				{
					Name: "worker-name",
					Machine: garden.Machine{
						Type: "machine-type-1",
					},
					Minimum: 1,
					Maximum: 1,
					Volume: &garden.Volume{
						Size: "10Gi",
						Type: "volume-type-1",
					},
					Zones: []string{"europe-a"},
				},
			}

			BeforeEach(func() {
				cloudProfile = *cloudProfileBase.DeepCopy()
				shoot = *shootBase.DeepCopy()
				shoot.Spec.Provider.Workers = workers
			})

			It("should pass because no seed has to be specified (however can be). The scheduler sets the seed instead.", func() {
				shoot.Spec.SeedName = nil
				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject because the shoot node and the seed node networks intersect", func() {
				shoot.Spec.Networking.Nodes = seedNodesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot pod and the seed pod networks intersect", func() {
				shoot.Spec.Networking.Pods = &seedPodsCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the shoot service and the seed service networks intersect", func() {
				shoot.Spec.Networking.Services = &seedServicesCIDR

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is already used by another shoot", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is a subdomain of a domain already used by another shoot", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				subdomain := fmt.Sprintf("subdomain.%s", *anotherShoot.Spec.DNS.Domain)
				shoot.Spec.DNS.Domain = &subdomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is a subdomain of a domain already used by another shoot (case one)", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				subdomain := fmt.Sprintf("subdomain.%s", *anotherShoot.Spec.DNS.Domain)
				shoot.Spec.DNS.Domain = &subdomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject because the specified domain is a subdomain of a domain already used by another shoot (case two)", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				shoot.Spec.DNS.Domain = &baseDomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should allow because the specified domain is not a subdomain of a domain already used by another shoot", func() {
				anotherShoot := shoot.DeepCopy()
				anotherShoot.Name = "another-shoot"

				anotherDomain := fmt.Sprintf("someprefix%s", *anotherShoot.Spec.DNS.Domain)
				shoot.Spec.DNS.Domain = &anotherDomain

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(anotherShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(BeNil())
			})

			It("should reject due to an invalid kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.2.3"

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should default a major.minor kubernetes version to latest patch version", func() {
				shoot.Spec.Kubernetes.Version = "1.6"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.6.6"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, highestPatchVersion, garden.ExpirableVersion{Version: "1.7.1"}, garden.ExpirableVersion{Version: "1.7.2"})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(Not(HaveOccurred()))
				Expect(shoot.Spec.Kubernetes.Version).To(Equal(highestPatchVersion.Version))
			})

			It("should reject: default only exactly matching minor kubernetes version", func() {
				shoot.Spec.Kubernetes.Version = "1.8"
				highestPatchVersion := garden.ExpirableVersion{Version: "1.81.5"}
				cloudProfile.Spec.Kubernetes.Versions = append(cloudProfile.Spec.Kubernetes.Versions, garden.ExpirableVersion{Version: "1.81.0"}, highestPatchVersion)

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine image", func() {
				shoot.Spec.Provider.Workers[0].Machine.Image = &garden.ShootMachineImage{
					Name:    "not-supported",
					Version: "not-supported",
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to a machine image with expiration date in the past", func() {
				imageVersionExpired := "0.0.1-beta"

				shoot.Spec.Provider.Workers[0].Machine.Image = &garden.ShootMachineImage{
					Name:    validMachineImageName,
					Version: imageVersionExpired,
				}

				timeInThePast := metav1.Now().Add(time.Second * -1000)
				cloudProfile.Spec.MachineImages = append(cloudProfile.Spec.MachineImages, garden.CloudProfileMachineImage{
					Name: validMachineImageName,
					Versions: []garden.ExpirableVersion{
						{
							Version:        imageVersionExpired,
							ExpirationDate: &metav1.Time{Time: timeInThePast},
						},
					},
				}, garden.CloudProfileMachineImage{
					Name: "other-image-name",
					Versions: []garden.ExpirableVersion{
						{
							Version: imageVersionExpired,
						},
					},
				})

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should not reject due to an usable machine type", func() {
				shoot.Spec.Provider.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject due to a not usable machine type", func() {
				shoot.Spec.Provider.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-old",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid machine type", func() {
				shoot.Spec.Provider.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid volume type", func() {
				shoot.Spec.Provider.Workers = []garden.Worker{
					{
						Machine: garden.Machine{
							Type: "machine-type-1",
						},
						Volume: &garden.Volume{
							Type: "not-allowed",
						},
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should reject due to an invalid zone", func() {
				shoot.Spec.Provider.Workers[0].Zones = []string{"invalid-zone"}

				gardenInformerFactory.Garden().InternalVersion().Projects().Informer().GetStore().Add(&project)
				gardenInformerFactory.Garden().InternalVersion().CloudProfiles().Informer().GetStore().Add(&cloudProfile)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})
	})
})
