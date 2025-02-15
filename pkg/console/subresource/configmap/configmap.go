package configmap

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/console-operator/pkg/api"
	"github.com/openshift/console-operator/pkg/console/subresource/consoleserver"
	routesub "github.com/openshift/console-operator/pkg/console/subresource/route"
	"github.com/openshift/console-operator/pkg/console/subresource/util"
)

const (
	consoleConfigYamlFile = "console-config.yaml"
	defaultLogoutURL      = ""
)

func getApiUrl(infrastructureConfig *configv1.Infrastructure) string {
	if infrastructureConfig != nil {
		return infrastructureConfig.Status.APIServerURL
	}
	return ""
}

func statusPageId(operatorConfig *operatorv1.Console) string {
	if operatorConfig.Spec.Providers.Statuspage != nil {
		return operatorConfig.Spec.Providers.Statuspage.PageID
	}
	return ""
}

func DefaultConfigMap(
	operatorConfig *operatorv1.Console,
	consoleConfig *configv1.Console,
	managedConfig *corev1.ConfigMap,
	monitoringSharedConfig *corev1.ConfigMap,
	infrastructureConfig *configv1.Infrastructure,
	activeConsoleRoute *routev1.Route,
	useDefaultCAFile bool,
	inactivityTimeoutSeconds int,
	pluginsEndpoingMap map[string]string) (consoleConfigmap *corev1.ConfigMap, unsupportedOverridesHaveMerged bool, err error) {

	defaultBuilder := &consoleserver.ConsoleServerCLIConfigBuilder{}
	defaultConfig, err := defaultBuilder.Host(activeConsoleRoute.Spec.Host).
		LogoutURL(defaultLogoutURL).
		Brand(DEFAULT_BRAND).
		DocURL(DEFAULT_DOC_URL).
		DefaultIngressCert(useDefaultCAFile).
		APIServerURL(getApiUrl(infrastructureConfig)).
		Monitoring(monitoringSharedConfig).
		InactivityTimeout(inactivityTimeoutSeconds).
		ConfigYAML()
	if err != nil {
		klog.Errorf("failed to generate default console-config config: %v", err)
		return nil, false, err
	}

	extractedManagedConfig := extractYAML(managedConfig)
	userDefinedBuilder := &consoleserver.ConsoleServerCLIConfigBuilder{}
	userDefinedConfig, err := userDefinedBuilder.Host(activeConsoleRoute.Spec.Host).
		LogoutURL(consoleConfig.Spec.Authentication.LogoutRedirect).
		Brand(operatorConfig.Spec.Customization.Brand).
		DocURL(operatorConfig.Spec.Customization.DocumentationBaseURL).
		DefaultIngressCert(useDefaultCAFile).
		APIServerURL(getApiUrl(infrastructureConfig)).
		Monitoring(monitoringSharedConfig).
		Plugins(pluginsEndpoingMap).
		CustomLogoFile(operatorConfig.Spec.Customization.CustomLogoFile.Key).
		CustomProductName(operatorConfig.Spec.Customization.CustomProductName).
		CustomDeveloperCatalog(operatorConfig.Spec.Customization.DeveloperCatalog).
		CustomHostnameRedirectPort(routesub.IsCustomRouteSet(operatorConfig)).
		StatusPageID(statusPageId(operatorConfig)).
		InactivityTimeout(inactivityTimeoutSeconds).
		ConfigYAML()
	if err != nil {
		klog.Errorf("failed to generate user defined console-config config: %v", err)
		return nil, false, err
	}

	unsupportedConfigOverride := operatorConfig.Spec.UnsupportedConfigOverrides.Raw
	willMergeConfigOverrides := len(unsupportedConfigOverride) != 0
	if willMergeConfigOverrides {
		klog.V(4).Infoln(fmt.Sprintf("with UnsupportedConfigOverrides: %v", string(unsupportedConfigOverride)))
	}

	merger := &consoleserver.ConsoleYAMLMerger{}
	mergedConfig, err := merger.Merge(
		defaultConfig,
		extractedManagedConfig,
		userDefinedConfig,
		unsupportedConfigOverride)
	if err != nil {
		klog.Errorf("failed to generate configmap: %v", err)
		return nil, false, err
	}

	configMap := Stub()
	configMap.Data = map[string]string{}
	configMap.Data[consoleConfigYamlFile] = string(mergedConfig)
	util.AddOwnerRef(configMap, util.OwnerRefFrom(operatorConfig))

	return configMap, willMergeConfigOverrides, nil
}

func DefaultPublicConfig(consoleURL string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      api.OpenShiftConsolePublicConfigMapName,
			Namespace: api.OpenShiftConfigManagedNamespace,
		},
		Data: map[string]string{
			"consoleURL": consoleURL,
		},
	}
}

func EmptyPublicConfig() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      api.OpenShiftConsolePublicConfigMapName,
			Namespace: api.OpenShiftConfigManagedNamespace,
		},
		Data: map[string]string{},
	}
}

func Stub() *corev1.ConfigMap {
	meta := util.SharedMeta()
	meta.Name = api.OpenShiftConsoleConfigMapName
	configMap := &corev1.ConfigMap{
		ObjectMeta: meta,
	}
	return configMap
}

func consoleBaseAddr(host string) string {
	return util.HTTPS(host)
}

// Helper function that pulls the yaml struct out of the data section of a configmap yaml
func extractYAML(managedConfig *corev1.ConfigMap) []byte {
	data := managedConfig.Data
	for _, v := range data {
		return []byte(v)
	}

	return []byte{}
}
