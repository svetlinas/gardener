<%
  import os, yaml

  values={}
  if context.get("values", "") != "":
    values=yaml.load(open(context.get("values", "")), Loader=yaml.Loader)

  if context.get("cloud", "") == "":
    raise Exception("missing --var cloud={aws,azure,gcp,alicloud,openstack,packet} flag")

  def value(path, default):
    keys=str.split(path, ".")
    root=values
    for key in keys:
      if isinstance(root, dict):
        if key in root:
          root=root[key]
        else:
          return default
      else:
        return default
    return root

  region=""
  kubernetesVersion=""
  if cloud == "aws":
    region="eu-west-1"
    kubernetesVersion="1.16.0"
  elif cloud == "azure" or cloud == "az":
    region="westeurope"
    kubernetesVersion="1.16.0"
  elif cloud == "gcp":
    region="europe-west1"
    kubernetesVersion="1.16.0"
  elif cloud == "alicloud":
    region="cn-beijing"
    kubernetesVersion="1.16.0"
  elif cloud == "packet":
    region="ewr1"
    kubernetesVersion="1.16.0"
  elif cloud == "openstack" or cloud == "os":
    region="europe-1"
    kubernetesVersion="1.16.0"
%>---
apiVersion: garden.sapcloud.io/v1beta1
kind: Shoot
metadata:
  name: ${value("metadata.name", "johndoe-" + cloud)}
  namespace: ${value("metadata.namespace", "garden-dev")}<% annotations = value("metadata.annotations", {}); labels = value("metadata.labels", {}) %>
  % if annotations != {}:
  annotations: ${yaml.dump(annotations, width=10000, default_flow_style=None)}
  % endif
  % if labels != {}:
  labels: ${yaml.dump(labels, width=10000, default_flow_style=None)}
  % endif
spec:
  cloud:
    profile: ${value("spec.cloud.profile", cloud)}
    region: ${value("spec.cloud.region", region)}
    secretBindingRef:
      name: ${value("spec.cloud.secretBindingRef.name", "core-" + cloud)}
    % if cloud == "aws":
    aws:
    # machineImage: # this machine image is default machine image for all worker pools
    #   name: coreos
    #   version: 2023.5.0
      networks:
        vpc:<% vpcID = value("spec.cloud.aws.networks.vpc.id", ""); vpcCIDR = value("spec.cloud.aws.networks.vpc.cidr", "10.250.0.0/16") %> # specify either 'id' or 'cidr'
          % if vpcID != "":
          id: ${vpcID}
        # cidr: 10.250.0.0/16
          % else:
        # id: vpc-123456
          cidr: ${vpcCIDR}
          % endif
        internal: ${value("spec.cloud.aws.networks.internal", ["10.250.112.0/22"])}
        public: ${value("spec.cloud.aws.networks.public", ["10.250.96.0/22"])}
        workers: ${value("spec.cloud.aws.networks.workers", ["10.250.0.0/19"])}
      workers:<% workers=value("spec.cloud.aws.workers", []) %>
      % if workers != []:
      ${yaml.dump(workers, width=10000, default_flow_style=None)}
      % else:
      - name: cpu-worker
        machineType: m5.large
        volumeType: gp2
        volumeSize: 20Gi
        autoScalerMin: 2
        autoScalerMax: 2
        maxSurge: 1
        maxUnavailable: 0
      # kubelet:<% kubelet=value("spec.cloud.aws.workers.kubelet", {}) %>
        % if kubelet != {}:
        kubelet:
        ${yaml.dump(kubelet, width=10000, default_flow_style=None)}
        % else:
        # cpuCFSQuota: true
        # cpuManagerPolicy: none
        # podPidsLimit: 10
        # maxPods: 110
        # evictionPressureTransitionPeriod: 4m0s
        # evictionMaxPodGracePeriod: 90
        # evictionHard:
        #   memoryAvailable: 100Mi
        #   imageFSAvailable: 5%
        #   imageFSInodesFree: 5%
        #   nodeFSAvailable: 5%
        #   nodeFSInodesFree: 5%
        # evictionSoft:
        #   memoryAvailable: 200Mi
        #   imageFSAvailable: 10%
        #   imageFSInodesFree: 10%
        #   nodeFSAvailable: 10%
        #   nodeFSInodesFree: 10%
        # evictionSoftGracePeriod:
        #   memoryAvailable: 1m30s
        #   imageFSAvailable: 1m30s
        #   imageFSInodesFree: 1m30s
        #   nodeFSAvailable: 1m30s
        #   nodeFSInodesFree: 1m30s
        # evictionMinimumReclaim:
        #   memoryAvailable: 0Mi
        #   imageFSAvailable: 0Mi
        #   imageFSInodesFree: 0Mi
        #   nodeFSAvailable: 0Mi
        #   nodeFSInodesFree: 0Mi
        # featureGates:
        #   SomeKubernetesFeature: true
          % endif
      # machineImage:
      #   name: coreos
      #   version: 2023.5.0
      # labels:
      #   key: value
      # annotations:
      #   key: value
      # taints: # See also https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
      # - key: foo
      #   value: bar
      #   effect: NoSchedule
      % endif
      zones: ${value("spec.cloud.aws.zones", ["eu-west-1a"])}
    % endif
    % if cloud == "azure" or cloud == "az":
    azure:<% resourceGroupName = value("spec.cloud.azure.resourceGroup.name", "") %>
      % if resourceGroupName != "":
      resourceGroup:
        name: ${resourceGroup}
      % else:
    # resourceGroup:
    #   name: mygroup
      % endif
    # machineImage: # this machine image is default machine image for all worker pools
    #   name: coreos
    #   version: 2023.5.0
      networks:
        vnet:<% vnetName = value("spec.cloud.azure.networks.vnet.name", ""); vnetCIDR = value("spec.cloud.azure.networks.vnet.cidr", "10.250.0.0/16") %> # specify either 'name' or 'cidr'
          % if vnetName != "":
          name: ${vnetName}
        # cidr: 10.250.0.0/16
          % else:
        # name: my-vnet
          cidr: ${vnetCIDR}
          % endif
        workers: ${value("spec.cloud.azure.networks.workers", "10.250.0.0/19")}
      workers:<% workers=value("spec.cloud.azure.workers", []) %>
      % if workers != []:
      ${yaml.dump(workers, width=10000, default_flow_style=None)}
      % else:
      - name: cpu-worker
        machineType: Standard_D2_v3
        volumeType: standard
        volumeSize: 35Gi # must be at least 35Gi for Azure VMs
        autoScalerMin: 2
        autoScalerMax: 2
        maxSurge: 1
        maxUnavailable: 0
      # kubelet:<% kubelet=value("spec.cloud.azure.workers.kubelet", {}) %>
        % if kubelet != {}:
        kubelet:
        ${yaml.dump(kubelet, width=10000, default_flow_style=None)}
        % else:
        # cpuCFSQuota: true
        # cpuManagerPolicy: none
        # podPidsLimit: 10
        # maxPods: 110
        # evictionPressureTransitionPeriod: 4m0s
        # evictionMaxPodGracePeriod: 90
        # evictionHard:
        #   memoryAvailable: 100Mi
        #   imageFSAvailable: 5%
        #   imageFSInodesFree: 5%
        #   nodeFSAvailable: 5%
        #   nodeFSInodesFree: 5%
        # evictionSoft:
        #   memoryAvailable: 200Mi
        #   imageFSAvailable: 10%
        #   imageFSInodesFree: 10%
        #   nodeFSAvailable: 10%
        #   nodeFSInodesFree: 10%
        # evictionSoftGracePeriod:
        #   memoryAvailable: 1m30s
        #   imageFSAvailable: 1m30s
        #   imageFSInodesFree: 1m30s
        #   nodeFSAvailable: 1m30s
        #   nodeFSInodesFree: 1m30s
        # evictionMinimumReclaim:
        #   memoryAvailable: 0Mi
        #   imageFSAvailable: 0Mi
        #   imageFSInodesFree: 0Mi
        #   nodeFSAvailable: 0Mi
        #   nodeFSInodesFree: 0Mi
        # featureGates:
        #   SomeKubernetesFeature: true
          % endif
      # machineImage:
      #   name: coreos
      #   version: 2023.5.0
      # labels:
      #   key: value
      # annotations:
      #   key: value
      # taints: # See also https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
      # - key: foo
      #   value: bar
      #   effect: NoSchedule
      % endif
    % endif
    % if cloud == "alicloud":
    alicloud:
    # machineImage: # this machine image is default machine image for all worker pools
    #   name: coreos-alicloud
    #   version: 2023.5.0
      networks:
        vpc:<% vpcID = value("spec.cloud.alicloud.networks.vpc.id", ""); vpcCIDR = value("spec.cloud.alicloud.networks.vpc.cidr", "10.250.0.0/16") %> # specify either 'id' or 'cidr'
          % if vpcID != "":
          id: ${vpcID}
          # cidr: 10.250.0.0/16
          % else:
          # id: vpc-123456
          cidr: ${vpcCIDR}
          % endif
        workers: ${value("spec.cloud.alicloud.networks.workers", ["10.250.0.0/19"])}
      workers:<% workers=value("spec.cloud.alicloud.workers", []) %>
      % if workers != []:
      ${yaml.dump(workers, width=10000, default_flow_style=None)}
      % else:
      - name: small
        machineType: ecs.sn2ne.xlarge
        volumeType: cloud_efficiency
        volumeSize: 30Gi
        autoScalerMin: 2
        autoScalerMax: 2
        maxSurge: 1
        maxUnavailable: 0
      # kubelet:<% kubelet=value("spec.cloud.alicloud.workers.kubelet", {}) %>
        % if kubelet != {}:
        kubelet:
        ${yaml.dump(kubelet, width=10000, default_flow_style=None)}
        % else:
        # cpuCFSQuota: true
        # cpuManagerPolicy: none
        # podPidsLimit: 10
        # maxPods: 110
        # evictionPressureTransitionPeriod: 4m0s
        # evictionMaxPodGracePeriod: 90
        # evictionHard:
        #   memoryAvailable: 100Mi
        #   imageFSAvailable: 5%
        #   imageFSInodesFree: 5%
        #   nodeFSAvailable: 5%
        #   nodeFSInodesFree: 5%
        # evictionSoft:
        #   memoryAvailable: 200Mi
        #   imageFSAvailable: 10%
        #   imageFSInodesFree: 10%
        #   nodeFSAvailable: 10%
        #   nodeFSInodesFree: 10%
        # evictionSoftGracePeriod:
        #   memoryAvailable: 1m30s
        #   imageFSAvailable: 1m30s
        #   imageFSInodesFree: 1m30s
        #   nodeFSAvailable: 1m30s
        #   nodeFSInodesFree: 1m30s
        # evictionMinimumReclaim:
        #   memoryAvailable: 0Mi
        #   imageFSAvailable: 0Mi
        #   imageFSInodesFree: 0Mi
        #   nodeFSAvailable: 0Mi
        #   nodeFSInodesFree: 0Mi
        # featureGates:
        #   SomeKubernetesFeature: true
          % endif
      # machineImage:
      #   name: coreos-alicloud
      #   version: 2023.5.0
      # labels:
      #   key: value
      # annotations:
      #   key: value
      # taints: # See also https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
      # - key: foo
      #   value: bar
      #   effect: NoSchedule
      % endif
      zones: ${value("spec.cloud.alicloud.zones", ["cn-beijing-f"])}
    % endif
    % if cloud == "packet":
    packet:
    # machineImage: # this machine image is default machine image for all worker pools
    #   name: coreos
    #   version: 2079.3.0
      workers:<% workers=value("spec.cloud.packet.workers", []) %>
      % if workers != []:
      ${yaml.dump(workers, width=10000, default_flow_style=None)}
      % else:
      - name: small
        machineType: c1.small
        volumeType: standard
        volumeSize: 30Gi
        autoScalerMin: 1
        autoScalerMax: 2
        maxSurge: 1
        maxUnavailable: 0
      # kubelet:<% kubelet=value("spec.cloud.packet.workers.kubelet", {}) %>
        % if kubelet != {}:
        kubelet:
        ${yaml.dump(kubelet, width=10000, default_flow_style=None)}
        % else:
        # cpuCFSQuota: true
        # cpuManagerPolicy: none
        # podPidsLimit: 10
        # maxPods: 110
        # evictionPressureTransitionPeriod: 4m0s
        # evictionMaxPodGracePeriod: 90
        # evictionHard:
        #   memoryAvailable: 100Mi
        #   imageFSAvailable: 5%
        #   imageFSInodesFree: 5%
        #   nodeFSAvailable: 5%
        #   nodeFSInodesFree: 5%
        # evictionSoft:
        #   memoryAvailable: 200Mi
        #   imageFSAvailable: 10%
        #   imageFSInodesFree: 10%
        #   nodeFSAvailable: 10%
        #   nodeFSInodesFree: 10%
        # evictionSoftGracePeriod:
        #   memoryAvailable: 1m30s
        #   imageFSAvailable: 1m30s
        #   imageFSInodesFree: 1m30s
        #   nodeFSAvailable: 1m30s
        #   nodeFSInodesFree: 1m30s
        # evictionMinimumReclaim:
        #   memoryAvailable: 0Mi
        #   imageFSAvailable: 0Mi
        #   imageFSInodesFree: 0Mi
        #   nodeFSAvailable: 0Mi
        #   nodeFSInodesFree: 0Mi
        # featureGates:
        #   SomeKubernetesFeature: true
          % endif
      # machineImage:
      #   name: coreos
      #   version: 2079.3.0
      # labels:
      #   key: value
      # annotations:
      #   key: value
      # taints: # See also https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
      # - key: foo
      #   value: bar
      #   effect: NoSchedule
      % endif
      zones: ${value("spec.cloud.packet.zones", ["ewr1"])}
    % endif
    % if cloud == "gcp":
    gcp:
    # machineImage: # this machine image is default machine image for all worker pools
    #   name: coreos
    #   version: 2023.5.0
      networks:<% vpcName = value("spec.cloud.gcp.networks.vpc.name", "") %>
      % if vpcName != "":
        vpc:
          name: ${vpcName}
      % else:
      # vpc:
      #   name: my-vpc
      % endif
        internal: ${value("spec.cloud.gcp.networks.internal", "10.250.112.0/22")}
        workers: ${value("spec.cloud.gcp.networks.workers", ["10.250.0.0/19"])}
      workers:<% workers=value("spec.cloud.gcp.workers", []) %>
      % if workers != []:
      ${yaml.dump(workers, width=10000, default_flow_style=None)}
      % else:
      - name: cpu-worker
        machineType: n1-standard-4
        volumeType: pd-standard
        volumeSize: 20Gi
        autoScalerMin: 2
        autoScalerMax: 2
        maxSurge: 1
        maxUnavailable: 0
      # kubelet:<% kubelet=value("spec.cloud.gcp.workers.kubelet", {}) %>
        % if kubelet != {}:
        kubelet:
        ${yaml.dump(kubelet, width=10000, default_flow_style=None)}
        % else:
        # cpuCFSQuota: true
        # cpuManagerPolicy: none
        # podPidsLimit: 10
        # maxPods: 110
        # evictionPressureTransitionPeriod: 4m0s
        # evictionMaxPodGracePeriod: 90
        # evictionHard:
        #   memoryAvailable: 100Mi
        #   imageFSAvailable: 5%
        #   imageFSInodesFree: 5%
        #   nodeFSAvailable: 5%
        #   nodeFSInodesFree: 5%
        # evictionSoft:
        #   memoryAvailable: 200Mi
        #   imageFSAvailable: 10%
        #   imageFSInodesFree: 10%
        #   nodeFSAvailable: 10%
        #   nodeFSInodesFree: 10%
        # evictionSoftGracePeriod:
        #   memoryAvailable: 1m30s
        #   imageFSAvailable: 1m30s
        #   imageFSInodesFree: 1m30s
        #   nodeFSAvailable: 1m30s
        #   nodeFSInodesFree: 1m30s
        # evictionMinimumReclaim:
        #   memoryAvailable: 0Mi
        #   imageFSAvailable: 0Mi
        #   imageFSInodesFree: 0Mi
        #   nodeFSAvailable: 0Mi
        #   nodeFSInodesFree: 0Mi
        # featureGates:
        #   SomeKubernetesFeature: true
          % endif
      # machineImage:
      #   name: coreos
      #   version: 2023.5.0
      # labels:
      #   key: value
      # annotations:
      #   key: value
      # taints: # See also https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
      # - key: foo
      #   value: bar
      #   effect: NoSchedule
      % endif
      zones: ${value("spec.cloud.gcp.zones", ["europe-west1-b"])}
    % endif
    % if cloud == "openstack" or cloud == "os":
    openstack:
      loadBalancerProvider: ${value("spec.cloud.openstack.loadBalancerProvider", "haproxy")}
      floatingPoolName: ${value("spec.cloud.openstack.floatingPoolName", "MY-FLOATING-POOL")}
    # machineImage: # this machine image is default machine image for all worker pools
    #   name: coreos
    #   version: 2023.5.0
      networks:<% routerID = value("spec.cloud.openstack.networks.router.id", "") %>
      % if routerID != "":
        router:
          id: ${routerID}
      % else:
      # router:
      #   id: 1234
      % endif
        workers: ${value("spec.cloud.openstack.networks.workers", ["10.250.0.0/19"])}
      workers:<% workers=value("spec.cloud.openstack.workers", []) %>
      % if workers != []:
      ${yaml.dump(workers, width=10000, default_flow_style=None)}
      % else:
      - name: cpu-worker
        machineType: medium_2_4
        autoScalerMin: 2
        autoScalerMax: 2
        maxSurge: 1
        maxUnavailable: 0
      # kubelet:<% kubelet=value("spec.cloud.openstack.workers.kubelet", {}) %>
        % if kubelet != {}:
        kubelet:
        ${yaml.dump(kubelet, width=10000, default_flow_style=None)}
        % else:
        # cpuCFSQuota: true
        # cpuManagerPolicy: none
        # podPidsLimit: 10
        # maxPods: 110
        # evictionPressureTransitionPeriod: 4m0s
        # evictionMaxPodGracePeriod: 90
        # evictionHard:
        #   memoryAvailable: 100Mi
        #   imageFSAvailable: 5%
        #   imageFSInodesFree: 5%
        #   nodeFSAvailable: 5%
        #   nodeFSInodesFree: 5%
        # evictionSoft:
        #   memoryAvailable: 200Mi
        #   imageFSAvailable: 10%
        #   imageFSInodesFree: 10%
        #   nodeFSAvailable: 10%
        #   nodeFSInodesFree: 10%
        # evictionSoftGracePeriod:
        #   memoryAvailable: 1m30s
        #   imageFSAvailable: 1m30s
        #   imageFSInodesFree: 1m30s
        #   nodeFSAvailable: 1m30s
        #   nodeFSInodesFree: 1m30s
        # evictionMinimumReclaim:
        #   memoryAvailable: 0Mi
        #   imageFSAvailable: 0Mi
        #   imageFSInodesFree: 0Mi
        #   nodeFSAvailable: 0Mi
        #   nodeFSInodesFree: 0Mi
        # featureGates:
        #   SomeKubernetesFeature: true
          % endif
      # machineImage:
      #   name: coreos
      #   version: 2023.5.0
      # labels:
      #   key: value
      # annotations:
      #   key: value
      # taints: # See also https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/
      # - key: foo
      #   value: bar
      #   effect: NoSchedule
      % endif
      zones: ${value("spec.cloud.openstack.zones", ["europe-1a"])}
    % endif
  kubernetes:
  # clusterAutoscaler:
  #   scaleDownUtilizationThreshold: 0.5
  #   scaleDownUnneededTime: 30m
  #   scaleDownDelayAfterAdd: 60m
  #   scaleDownDelayAfterFailure: 10m
  #   scaleDownDelayAfterDelete: 10s
  #   scanInterval: 10s
    version: ${value("spec.kubernetes.version", kubernetesVersion)}<% kubeAPIServer=value("spec.kubernetes.kubeAPIServer", {}) %><% cloudControllerManager=value("spec.kubernetes.cloudControllerManager", {}) %><% kubeControllerManager=value("spec.kubernetes.kubeControllerManager", {}) %><% kubeScheduler=value("spec.kubernetes.kubeScheduler", {}) %><% kubeProxy=value("spec.kubernetes.kubeProxy", {}) %><% kubelet=value("spec.kubernetes.kubelet", {}) %> # specify "major.minor" to get latest patch version
    allowPrivilegedContainers: ${value("spec.kubernetes.allowPrivilegedContainers", "true")} # 'true' means that all authenticated users can use the "gardener.privileged" PodSecurityPolicy, allowing full unrestricted access to Pod features.
    % if kubeAPIServer != {}:
    kubeAPIServer: ${yaml.dump(kubeAPIServer, width=10000, default_flow_style=None)}
    % else:
  # kubeAPIServer:
  #   admissionPlugins:
  #   - name: PodNodeSelector
  #     config:
  #       podNodeSelectorPluginConfig:
  #         clusterDefaultNodeSelector: <node-selectors-labels>
  #         namespace1: <node-selectors-labels>
  #         namespace2: <node-selectors-labels>
  #   auditConfig:
  #     auditPolicy:
  #       configMapRef:
  #         name: auditpolicy
  #   enableBasicAuthentication: true
  #   featureGates:
  #     SomeKubernetesFeature: true
  #   oidcConfig:
  #     caBundle: |
  #       -----BEGIN CERTIFICATE-----
  #       Li4u
  #       -----END CERTIFICATE-----
  #     clientID: client-id
  #     groupsClaim: groups-claim
  #     groupsPrefix: groups-prefix
  #     issuerURL: https://identity.example.com
  #     usernameClaim: username-claim
  #     usernamePrefix: username-prefix
  #     signingAlgs: [RS256,some-other-algorithm]
  #-#-# only usable with Kubernetes >= 1.11
  #     requiredClaims:
  #       key: value
  #   runtimeConfig:
  #     scheduling.k8s.io/v1alpha1: true
  #-#-# requires TokenRequest feature gate
  #-#-# See https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
  #   serviceAccountConfig:
  #     issuer: "https://${value("spec.dns.domain", value("metadata.name", "johndoe-" + cloud) + "." + value("metadata.namespace", "garden-dev") + ".example.com")}"
  #     signingKeySecretName: "service-account-signing-key"
  #   apiAudiences: ["some", "audiences"]
  % endif
    % if cloudControllerManager != {}:
    cloudControllerManager: ${yaml.dump(cloudControllerManager, width=10000, default_flow_style=None)}
    % else:
  # cloudControllerManager:
  #   featureGates:
  #     SomeKubernetesFeature: true
  % endif
    % if kubeControllerManager != {}:
    kubeControllerManager: ${yaml.dump(kubeControllerManager, width=10000, default_flow_style=None)}
    % else:
  # kubeControllerManager:
  #   featureGates:
  #     SomeKubernetesFeature: true
  # The NodeCIRDMaskSize field is immutable due to https://github.com/kubernetes/kubernetes/issues/70957
  #   nodeCIDRMaskSize: 24
  #   horizontalPodAutoscaler:
  #     syncPeriod: 30s
  #     tolerance: 0.1
  #-#-# only usable with Kubernetes < 1.12
  #     downscaleDelay: 15m0s
  #     upscaleDelay: 1m0s
  #-#-# only usable with Kubernetes >= 1.12
  #     downscaleStabilization: 5m0s
  #     initialReadinessDelay: 30s
  #     cpuInitializationPeriod: 5m0s
  % endif
    % if kubeScheduler != {}:
    kubeScheduler: ${yaml.dump(kubeScheduler, width=10000, default_flow_style=None)}
    % else:
  # kubeScheduler:
  #   featureGates:
  #     SomeKubernetesFeature: true
  % endif
    % if kubeProxy != {}:
    kubeProxy: ${yaml.dump(kubeProxy, width=10000, default_flow_style=None)}
    % else:
  # kubeProxy:
  #   featureGates:
  #     SomeKubernetesFeature: true
  #   mode: IPVS
  % endif
    % if kubelet != {}:
    kubelet: ${yaml.dump(kubelet, width=10000, default_flow_style=None)}
    % else:
  # kubelet:
  #   cpuCFSQuota: true
  #   cpuManagerPolicy: none
  #   podPidsLimit: 10
  #   featureGates:
  #     SomeKubernetesFeature: true
  % endif
  dns:
    domain: ${value("spec.dns.domain", value("metadata.name", "johndoe-" + cloud) + "." + value("metadata.namespace", "garden-dev") + ".example.com")}<% hibernation = value("spec.hibernation", {}) %> # if not specified then Gardener will try to use the default domain for this shoot
  # provider: ${value("spec.dns.provider", "aws-route53")}     # only relevant if a custom domain is used for this shoot
  # secretName: my-dns-secret # only relevant if a custom domain is used for this shoot
  # includeZones: []          # only relevant if a custom domain is used for this shoot
  # excludeZones: []          # only relevant if a custom domain is used for this shoot
  % if hibernation != {}:
  hibernation: ${yaml.dump(hibernation, width=10000, default_flow_style=None)}
  % else:
# hibernation:
#   enabled: false
#   schedules:
#   - start: "0 20 * * *" # Start hibernation every day at 8PM
#     end: "0 6 * * *"    # Stop hibernation every day at 6AM
#     location: "America/Los_Angeles" # Specify a location for the cron to run in
  % endif
  maintenance:
    timeWindow:
      begin: ${value("spec.maintenance.timeWindow.begin", "220000+0100")}
      end: ${value("spec.maintenance.timeWindow.end", "230000+0100")}
    autoUpdate:
      kubernetesVersion: ${value("maintenance.autoUpdate.kubernetesVersion", "true")}
      machineImageVersion: ${value("maintenance.autoUpdate.machineImageVersion", "true")}
  addons:
    # nginx-ingress addon is still supported but deprecated.
    # This field will be removed in the future. You should deploy your own ingress controller
    # instead of enabling it here. You should not use this field anymore.
    nginx-ingress:
      enabled: ${value("spec.addons.nginx-ingress.enabled", "false")}
      loadBalancerSourceRanges: ${value("spec.addons.nginx-ingress.loadBalancerSourceRanges", [])}
    kubernetes-dashboard:
      enabled: ${value("spec.addons.kubernetes-dashboard.enabled", "true")}
    # authenticationMode: basic # allowed values: basic,token
  % if cloud == "aws":
    # kube2iam addon is still supported but deprecated.
    # This field will be removed in the future. You should deploy kube2iam as well as
    # the desired AWS IAM roles on your own instead of enabling it here. Please do not
    # use this field anymore.
    kube2iam:
      enabled: ${value("spec.addons.kube2iam.enabled", "true")}
      roles:<% roles=value("spec.addons.kube2iam.roles", []) %>
      % if roles != []:
      ${yaml.dump(roles, width=10000, default_flow_style=None)}
      % else:
      - name: ecr
        description: "Allow access to ECR repositories beginning with 'my-images/', and creation of new repositories"
        policy: |
          {
            "Version": "2012-10-17",
            "Statement": [
              {
                "Action": "ecr:*",
                "Effect": "Allow",
                "Resource": "arn:aws:ecr:eu-central-1:<%text>${account_id}</%text>:repository/my-images/*"
              },
              {
                "Action": [
                  "ecr:GetAuthorizationToken",
                  "ecr:CreateRepository"
                ],
                "Effect": "Allow",
                "Resource": "*"
              }
            ]
          }
      % endif
    % endif
    # Heapster addon is deprecated and no longer supported. Gardener deploys the Kubernetes metrics-server
    # into the kube-system namespace of shoots (cannot be turned off) for fetching metrics and enabling
    # horizontal pod auto-scaling.
    # This field will be removed in the future and is only kept for API compatibility reasons. It is not
    # evaluated or respected at all. Please do not use this field anymore.
    heapster:
      enabled: false
    # cluster-autoscaler addon is automatically enabled if at least one of the configured
    # worker pools (see above) uses max>min. You do not need to enable it separately anymore. Any value
    # you put here has no effect. This field will be removed in the future. Please do not use it anymore.
    cluster-autoscaler:
      enabled: ${value("spec.addons.cluster-autoscaler.enabled", "true")}
    # kube-lego addon is still supported but deprecated.
    # This field will be removed in the future. You should deploy your own kube-lego/cert-manager
    # instead of enabling it here. You should not use this field anymore.
    kube-lego:
      enabled: ${value("spec.addons.kube-lego.enabled", "false")}
      email: ${value("spec.addons.kube-lego.email", "john.doe@example.com")}
    # Monocular addon is deprecated and no longer supported.
    # This field will be removed in the future and is only kept for API compatibility reasons. It is not
    # evaluated or respected at all. You should deploy Monocular on your own instead of enabling it here.
    # Please do not use this field anymore.
    monocular:
      enabled: false
