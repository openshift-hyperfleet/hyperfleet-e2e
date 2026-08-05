package helper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	pubsubadmin "cloud.google.com/go/pubsub/v2/apiv1"
	pubsubpb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper/helm"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdapterDeploymentOptions contains configuration for deploying an adapter via Helm
type AdapterDeploymentOptions struct {
	ReleaseName  string
	ChartPath    string
	AdapterName  string
	ResourceType string
}

// AdapterDeploymentList tracks adapters deployed during the test run for cleanup.
// Keys are adapter names, values are resource types (e.g. "clusters", "nodepools").
type AdapterDeploymentList struct {
	mu    sync.RWMutex
	items map[string]string
}

func (l *AdapterDeploymentList) Add(adapterName, resourceType string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items[adapterName] = resourceType
}

func (l *AdapterDeploymentList) Snapshot() map[string]string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	snapshot := make(map[string]string, len(l.items))
	for k, v := range l.items {
		snapshot[k] = v
	}
	return snapshot
}

func InitAdapterDeploymentList() *AdapterDeploymentList {
	return &AdapterDeploymentList{
		items: make(map[string]string),
	}
}

// GenerateAdapterReleaseName generates a deterministic Helm release name for an adapter deployment.
// The release name format is: adapter-<resource_type>-<adapter_name>
// Deterministic naming allows helm upgrade --install to upgrade in place and avoids duplicate releases.
// The name is truncated to 48 characters to leave room for Helm's deployment/pod suffixes (Kubernetes has a 63-char limit).
// If truncation is needed, a deterministic hash is appended to prevent collisions between long names.
const maxReleaseNameLength = 48

func GenerateAdapterReleaseName(resourceType, adapterName string) string {
	releaseName := fmt.Sprintf("adapter-%s-%s", resourceType, adapterName)
	if len(releaseName) > maxReleaseNameLength {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(releaseName)))[:8]
		truncLen := maxReleaseNameLength - len(hash) - 1
		releaseName = releaseName[:truncLen] + "-" + hash
	}
	return releaseName
}

func (h *Helper) InstallAdapter(ctx context.Context, opts AdapterDeploymentOptions) error {
	if opts.AdapterName == "" {
		return fmt.Errorf("AdapterDeploymentOptions.AdapterName is required")
	}
	if opts.ReleaseName == "" {
		return fmt.Errorf("AdapterDeploymentOptions.ReleaseName is required")
	}
	if opts.ChartPath == "" {
		return fmt.Errorf("AdapterDeploymentOptions.ChartPath is required")
	}
	if opts.ResourceType == "" {
		return fmt.Errorf("AdapterDeploymentOptions.ResourceType is required")
	}

	data := map[string]interface{}{
		"BrokerType":       h.Cfg.BrokerType,
		"ProjectId":        h.Cfg.GCPProjectID,
		"Namespace":        h.Cfg.Namespace,
		"RunId":            h.Cfg.RunID,
		"AdapterName":      opts.AdapterName,
		"ImageRegistry":    h.Cfg.AdapterDeployment.ImageRegistry,
		"AdapterImageRepo": h.Cfg.AdapterDeployment.ImageRepo,
		"ImagePullPolicy":  os.Getenv("IMAGE_PULL_POLICY"),
		"AdapterImageTag":  h.Cfg.AdapterDeployment.ImageTag,
		"AdapterGooglepubsubCreateTopicIfMissing":        "true",
		"AdapterGooglepubsubCreateSubscriptionIfMissing": "true",
		"RabbitmqUrl": os.Getenv("RABBITMQ_URL"),
	}

	baseTemplateFilePath := fmt.Sprintf("%s/adapter-configs/%s-base.tmpl", h.Cfg.TestDataDir, opts.ResourceType)
	valuesFilePath := fmt.Sprintf("%s/adapter-configs/%s.yaml", h.Cfg.TestDataDir, opts.AdapterName)
	releaseValues, err := parseTemplateWithValues(data, baseTemplateFilePath, valuesFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse template with values for adapter %s: %w", opts.AdapterName, err)
	}

	releaseValues["fullnameOverride"] = opts.ReleaseName
	labels := map[string]string{
		"e2e.hyperfleet.io/run-id": h.Cfg.RunID,
	}

	logger.Info("Release Values", releaseValues)
	helmClient := helm.NewHelmClient(h.Cfg.Namespace)
	if err := helmClient.InstallRelease(ctx, opts.ReleaseName, opts.ChartPath, releaseValues, labels); err != nil {
		return fmt.Errorf("failed to install adapter %s (release %s): %w", opts.AdapterName, opts.ReleaseName, err)
	}

	h.AdapterDeploymentList.Add(opts.AdapterName, opts.ResourceType)

	logger.Info("adapter installed successfully",
		"adapter_name", opts.AdapterName,
		"release_name", opts.ReleaseName)

	return nil
}

func (h *Helper) UninstallAdapter(ctx context.Context, opts AdapterDeploymentOptions) error {
	var errs []error
	helmClient := helm.NewHelmClient(h.Cfg.Namespace)
	err := helmClient.UninstallRelease(opts.ReleaseName)
	if err != nil {
		logger.Error("failed to uninstall release", "release", opts.ReleaseName, "error", err)
		errs = append(errs, fmt.Errorf("uninstall release %s: %w", opts.ReleaseName, err))
	}
	h.cleanupClusterScopedResources(ctx, opts.ReleaseName)
	err = h.DeletePubSubResourcesForAdapter(ctx, opts.AdapterName, opts.ResourceType)
	if err != nil {
		logger.Error("failed to delete pubsub resources", "adapter", opts.AdapterName, "error", err)
		errs = append(errs, fmt.Errorf("delete pubsub resources for %s: %w", opts.AdapterName, err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to uninstall adapter and cleanup resources: %v", errs)
	}
	return nil
}

func parseTemplateWithValues(data map[string]interface{}, baseTemplateFilePath string, baseValuesFilePath string) (map[string]interface{}, error) {
	// Parse the base template file
	tmpl, err := template.ParseFiles(baseTemplateFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template file %s: %w", baseTemplateFilePath, err)
	}

	// Execute template and write to buffer
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// Parse the resulting YAML into a map
	values := make(map[string]interface{})
	if err := yaml.Unmarshal(buf.Bytes(), &values); err != nil {
		return nil, fmt.Errorf("failed to parse rendered YAML: %w", err)
	}

	// Unmarshal the base values filepath into the values map
	adapterConfig, err := os.ReadFile(filepath.Clean(baseValuesFilePath))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", baseValuesFilePath, err)
	}
	if err := yaml.Unmarshal(adapterConfig, &values); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", baseValuesFilePath, err)
	}

	return values, nil
}

// cleanupClusterScopedResources removes orphaned cluster-scoped resources that may be left
// after Helm uninstall. This is a best-effort cleanup and logs errors without failing.
// Uses label selectors instead of names so it works regardless of the chart's naming scheme.
func (h *Helper) cleanupClusterScopedResources(ctx context.Context, releaseName string) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName)

	// Try to delete ClusterRole
	clusterRoleCmd := exec.CommandContext(cmdCtx, "kubectl", "delete", "clusterrole", //nolint:gosec // labelSelector is derived from releaseName, not user input
		"-l", labelSelector,
		"--ignore-not-found=true")
	if output, err := clusterRoleCmd.CombinedOutput(); err != nil {
		logger.Info("could not delete ClusterRole (may not exist)",
			"release_name", releaseName,
			"output", string(output))
	} else {
		logger.Info("cleaned up ClusterRole", "release_name", releaseName)
	}

	// Try to delete ClusterRoleBinding
	clusterRoleBindingCmd := exec.CommandContext(cmdCtx, "kubectl", "delete", "clusterrolebinding", //nolint:gosec // labelSelector is derived from releaseName, not user input
		"-l", labelSelector,
		"--ignore-not-found=true")
	if output, err := clusterRoleBindingCmd.CombinedOutput(); err != nil {
		logger.Info("could not delete ClusterRoleBinding (may not exist)",
			"release_name", releaseName,
			"output", string(output))
	} else {
		logger.Info("cleaned up ClusterRoleBinding", "release_name", releaseName)
	}
}

// PurgeAdapterQueue purges all pending messages from the broker queue for the given adapter.
// This is used before deploying a test adapter to avoid processing a stale message backlog
// accumulated while the adapter was uninstalled between test runs.
//
// Broker dispatch is determined by the BROKER_TYPE environment variable:
//   - "googlepubsub" (default when unset): deletes the Pub/Sub subscription so the adapter
//     recreates it fresh (the chart sets createSubscriptionIfMissing: true via DeployAdapter).
//     Subscription name pattern: {namespace}-clusters-{adapterName}
//   - "rabbitmq": purges the queue via rabbitmqctl.
//     Queue name pattern: {namespace}-clusters-{adapterName}-CHANGE_ME
//     The "CHANGE_ME" suffix is the literal subscription_id in adapter-config.yaml files.
//
// If the queue/subscription does not exist or the broker is unreachable, this is a no-op.
func (h *Helper) PurgeAdapterQueue(ctx context.Context, adapterName string) error {
	brokerType := os.Getenv("BROKER_TYPE")
	if brokerType == "" {
		brokerType = "googlepubsub" // matches DeployAdapter's default
	}
	switch brokerType {
	case "googlepubsub":
		subscriptionID := fmt.Sprintf("%s-clusters-%s", h.Cfg.Namespace, adapterName)
		return DeletePubSubSubscription(ctx, subscriptionID, h.Cfg.GCPProjectID)
	case "rabbitmq":
		return h.purgeRabbitMQQueue(ctx, adapterName)
	default:
		return fmt.Errorf("unsupported broker type %q", brokerType)
	}
}

// purgeRabbitMQQueue is the RabbitMQ-specific implementation used by PurgeAdapterQueue.
func (h *Helper) purgeRabbitMQQueue(ctx context.Context, adapterName string) error {
	const (
		rabbitMQNamespace    = "rabbitmq"
		rabbitMQPodLabelKey  = "app"
		rabbitMQPodLabelVal  = "rabbitmq"
		brokerSubscriptionID = "CHANGE_ME" // literal subscription_id in adapter-config.yaml
	)

	queueName := fmt.Sprintf("%s-clusters-%s-%s", h.Cfg.Namespace, adapterName, brokerSubscriptionID)
	logger.Info("purging RabbitMQ adapter queue", "queue", queueName, "adapter", adapterName)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pods, err := h.K8sClient.CoreV1().Pods(rabbitMQNamespace).List(cmdCtx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", rabbitMQPodLabelKey, rabbitMQPodLabelVal),
	})
	if err != nil {
		logger.Error("failed to list RabbitMQ pods", "namespace", rabbitMQNamespace, "error", err)
		return fmt.Errorf("failed to list RabbitMQ pods in namespace %q: %w", rabbitMQNamespace, err)
	}
	if len(pods.Items) == 0 {
		logger.Info("no RabbitMQ pods found, skipping queue purge", "namespace", rabbitMQNamespace)
		return nil
	}

	podName := pods.Items[0].Name
	cmd := exec.CommandContext(cmdCtx, "kubectl", "exec", "-n", rabbitMQNamespace, // #nosec G204 -- podName and queueName from trusted config
		podName, "--", "rabbitmqctl", "purge_queue", queueName)

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "not_found") || strings.Contains(outputStr, "does not exist") {
			logger.Info("queue not found, nothing to purge", "queue", queueName)
			return nil
		}
		return fmt.Errorf("failed to purge RabbitMQ queue %s: %w (output: %s)", queueName, err, outputStr)
	}

	logger.Info("RabbitMQ queue purged successfully", "queue", queueName)
	return nil
}

var newPubSubDeleteFunc = func(ctx context.Context, projectID, subID string) (func(context.Context) error, func(), error) {
	client, err := pubsubadmin.NewSubscriptionAdminClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Pub/Sub admin client: %w", err)
	}
	subPath := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subID)
	deleteFn := func(ctx context.Context) error {
		return client.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{
			Subscription: subPath,
		})
	}
	return deleteFn, func() {
		if err := client.Close(); err != nil {
			logger.Info("failed to close Pub/Sub admin client", "error", err)
		}
	}, nil
}

// DeletePubSubResourcesForAdapter deletes Pub/Sub subscription and dlq topic for a given adapter.
func (h *Helper) DeletePubSubResourcesForAdapter(ctx context.Context, adapterName string, resourceType string) error {
	if h.Cfg.BrokerType != "googlepubsub" {
		logger.Info("skipping Pub/Sub subscription and topic deletion for non-Google Pub/Sub adapter", "adapter", adapterName)
		return nil
	}

	namespace := h.Cfg.Namespace
	projectID := h.Cfg.GCPProjectID
	subscriptionID := fmt.Sprintf("%s-%s-%s", namespace, resourceType, adapterName)
	topicID := fmt.Sprintf("%s-%s-%s-dlq", namespace, resourceType, adapterName)

	var errorList []string
	if err := DeletePubSubSubscription(ctx, subscriptionID, projectID); err != nil {
		errorList = append(errorList, subscriptionID)
	}
	if err := DeletePubSubTopic(ctx, topicID, projectID); err != nil {
		errorList = append(errorList, topicID)
	}
	if len(errorList) > 0 {
		return fmt.Errorf("failed to delete some Pub/Sub resources for adapter %s: %s", adapterName, strings.Join(errorList, ", "))
	}
	logger.Info("deleted Pub/Sub resources for adapter", "adapter", adapterName)
	return nil
}

// DeletePubSubSubscription deletes a Google Pub/Sub subscription using the Go SDK.
// If the subscription does not exist, it is treated as a no-op.
func DeletePubSubSubscription(ctx context.Context, subscriptionID string, projectID string) error {
	if projectID == "" {
		projectID = defaultGCPProjectID
	}

	logger.Info("deleting Pub/Sub subscription",
		"subscription", subscriptionID,
		"project", projectID)

	deleteFn, cleanup, err := newPubSubDeleteFunc(ctx, projectID, subscriptionID)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := deleteFn(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			logger.Info("Pub/Sub subscription not found, skipping deletion", "subscription", subscriptionID)
			return nil
		}
		logger.Error("failed to delete Pub/Sub subscription",
			"subscription", subscriptionID,
			"project", projectID,
			"error", err)
		return fmt.Errorf("failed to delete Pub/Sub subscription %s: %w", subscriptionID, err)
	}

	logger.Info("Pub/Sub subscription deleted successfully", "subscription", subscriptionID)
	return nil
}

var newPubSubTopicDeleteFunc = func(ctx context.Context, projectID, topicID string) (func(context.Context) error, func(), error) {
	client, err := pubsubadmin.NewTopicAdminClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Pub/Sub topic admin client: %w", err)
	}
	topicPath := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	deleteFn := func(ctx context.Context) error {
		return client.DeleteTopic(ctx, &pubsubpb.DeleteTopicRequest{
			Topic: topicPath,
		})
	}
	return deleteFn, func() {
		if err := client.Close(); err != nil {
			logger.Info("failed to close Pub/Sub topic admin client", "error", err)
		}
	}, nil
}

// DeletePubSubTopic deletes a Google Pub/Sub topic using the Go SDK.
// If the topic does not exist, it is treated as a no-op.
func DeletePubSubTopic(ctx context.Context, topicID string, projectID string) error {
	if projectID == "" {
		projectID = defaultGCPProjectID
	}

	logger.Info("deleting Pub/Sub topic",
		"topic", topicID,
		"project", projectID)

	deleteFn, cleanup, err := newPubSubTopicDeleteFunc(ctx, projectID, topicID)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := deleteFn(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			logger.Info("Pub/Sub topic not found, skipping deletion", "topic", topicID)
			return nil
		}
		logger.Error("failed to delete Pub/Sub topic",
			"topic", topicID,
			"project", projectID,
			"error", err)
		return fmt.Errorf("failed to delete Pub/Sub topic %s: %w", topicID, err)
	}

	logger.Info("Pub/Sub topic deleted successfully", "topic", topicID)
	return nil
}
