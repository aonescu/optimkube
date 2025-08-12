package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// CostOptimizer main structure
type CostOptimizer struct {
	clientset       *kubernetes.Clientset
	metricsClient   *metricsclientset.Clientset
	costCalculator  *CostCalculator
	recommendations []Recommendation
}

// CostCalculator handles cost calculations
type CostCalculator struct {
	NodeCostPerHour map[string]float64 // instance type -> cost per hour
	StorageCostPerGB float64           // cost per GB per month
}

// NodeMetrics represents node resource usage
type NodeMetrics struct {
	Name            string  `json:"name"`
	CPUUsage        float64 `json:"cpu_usage"`
	MemoryUsage     float64 `json:"memory_usage"`
	CPUCapacity     float64 `json:"cpu_capacity"`
	MemoryCapacity  float64 `json:"memory_capacity"`
	CPUUtilization  float64 `json:"cpu_utilization"`
	MemoryUtilization float64 `json:"memory_utilization"`
	EstimatedCost   float64 `json:"estimated_cost"`
	InstanceType    string  `json:"instance_type"`
}

// PodMetrics represents pod resource usage
type PodMetrics struct {
	Name           string  `json:"name"`
	Namespace      string  `json:"namespace"`
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsage    float64 `json:"memory_usage"`
	CPURequest     float64 `json:"cpu_request"`
	MemoryRequest  float64 `json:"memory_request"`
	CPULimit       float64 `json:"cpu_limit"`
	MemoryLimit    float64 `json:"memory_limit"`
	EstimatedCost  float64 `json:"estimated_cost"`
}

// Recommendation represents optimization suggestions
type Recommendation struct {
	Type        string  `json:"type"`
	Resource    string  `json:"resource"`
	Namespace   string  `json:"namespace"`
	Description string  `json:"description"`
	Impact      string  `json:"impact"`
	Savings     float64 `json:"potential_savings"`
	Priority    string  `json:"priority"`
	Timestamp   time.Time `json:"timestamp"`
}

// ClusterCostSummary provides overall cost analysis
type ClusterCostSummary struct {
	TotalMonthlyCost     float64            `json:"total_monthly_cost"`
	ComputeCost          float64            `json:"compute_cost"`
	StorageCost          float64            `json:"storage_cost"`
	WastedResources      float64            `json:"wasted_resources"`
	PotentialSavings     float64            `json:"potential_savings"`
	NodeCount            int                `json:"node_count"`
	PodCount             int                `json:"pod_count"`
	NamespaceCosts       map[string]float64 `json:"namespace_costs"`
	RecommendationCount  int                `json:"recommendation_count"`
	LastUpdated          time.Time          `json:"last_updated"`
}

// OptimizationAction represents actions that can be taken
type OptimizationAction struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Resource    string    `json:"resource"`
	Namespace   string    `json:"namespace"`
	Action      string    `json:"action"`
	Parameters  map[string]interface{} `json:"parameters"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ExecutedAt  *time.Time `json:"executed_at,omitempty"`
}

func main() {
	optimizer, err := NewCostOptimizer()
	if err != nil {
		log.Fatalf("Failed to initialize cost optimizer: %v", err)
	}

	// Start background monitoring
	go optimizer.StartMonitoring()

	// Setup HTTP server
	router := mux.NewRouter()
	
	// API endpoints
	router.HandleFunc("/api/metrics/nodes", optimizer.handleNodeMetrics).Methods("GET")
	router.HandleFunc("/api/metrics/pods", optimizer.handlePodMetrics).Methods("GET")
	router.HandleFunc("/api/recommendations", optimizer.handleRecommendations).Methods("GET")
	router.HandleFunc("/api/cost-summary", optimizer.handleCostSummary).Methods("GET")
	router.HandleFunc("/api/optimize", optimizer.handleOptimize).Methods("POST")
	router.HandleFunc("/api/actions", optimizer.handleActions).Methods("GET")
	router.HandleFunc("/api/actions/{id}/execute", optimizer.handleExecuteAction).Methods("POST")

	// Health check
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}).Methods("GET")

	log.Println("Starting Kubernetes Cost Optimizer on :8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}

func NewCostOptimizer() (*CostOptimizer, error) {
	// Initialize Kubernetes client
	var config *rest.Config
	var err error

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	metricsClient, err := metricsclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %v", err)
	}

	// Initialize cost calculator with sample pricing
	costCalculator := &CostCalculator{
		NodeCostPerHour: map[string]float64{
			"t3.micro":    0.0104,
			"t3.small":    0.0208,
			"t3.medium":   0.0416,
			"t3.large":    0.0832,
			"t3.xlarge":   0.1664,
			"t3.2xlarge":  0.3328,
			"m5.large":    0.096,
			"m5.xlarge":   0.192,
			"m5.2xlarge":  0.384,
			"m5.4xlarge":  0.768,
			"c5.large":    0.085,
			"c5.xlarge":   0.17,
			"c5.2xlarge":  0.34,
			"c5.4xlarge":  0.68,
			"default":     0.1, // fallback cost
		},
		StorageCostPerGB: 0.10, // $0.10 per GB per month
	}

	return &CostOptimizer{
		clientset:       clientset,
		metricsClient:   metricsClient,
		costCalculator:  costCalculator,
		recommendations: make([]Recommendation, 0),
	}, nil
}

func (co *CostOptimizer) StartMonitoring() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		log.Println("Running cost analysis...")
		co.analyzeAndGenerateRecommendations()
		<-ticker.C
	}
}

func (co *CostOptimizer) analyzeAndGenerateRecommendations() {
	ctx := context.Background()
	recommendations := make([]Recommendation, 0)

	// Analyze nodes
	nodeRecommendations := co.analyzeNodes(ctx)
	recommendations = append(recommendations, nodeRecommendations...)

	// Analyze pods
	podRecommendations := co.analyzePods(ctx)
	recommendations = append(recommendations, podRecommendations...)

	// Analyze deployments
	deploymentRecommendations := co.analyzeDeployments(ctx)
	recommendations = append(recommendations, deploymentRecommendations...)

	co.recommendations = recommendations
	log.Printf("Generated %d recommendations", len(recommendations))
}

func (co *CostOptimizer) analyzeNodes(ctx context.Context) []Recommendation {
	recommendations := make([]Recommendation, 0)
	
	nodes, err := co.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list nodes: %v", err)
		return recommendations
	}

	nodeMetrics, err := co.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to get node metrics: %v", err)
		return recommendations
	}

	for _, node := range nodes.Items {
		// Find corresponding metrics
		var metrics *metricsv1beta1.NodeMetrics
		for _, m := range nodeMetrics.Items {
			if m.Name == node.Name {
				metrics = &m
				break
			}
		}

		if metrics == nil {
			continue
		}

		// Calculate utilization
		cpuCapacity := node.Status.Capacity[corev1.ResourceCPU]
		memoryCapacity := node.Status.Capacity[corev1.ResourceMemory]
		
		cpuUsage := metrics.Usage[corev1.ResourceCPU]
		memoryUsage := metrics.Usage[corev1.ResourceMemory]

		cpuUtil := float64(cpuUsage.MilliValue()) / float64(cpuCapacity.MilliValue()) * 100
		memoryUtil := float64(memoryUsage.Value()) / float64(memoryCapacity.Value()) * 100

		// Underutilized node recommendation
		if cpuUtil < 20 && memoryUtil < 30 {
			recommendations = append(recommendations, Recommendation{
				Type:        "node_optimization",
				Resource:    node.Name,
				Description: fmt.Sprintf("Node %s is underutilized (CPU: %.1f%%, Memory: %.1f%%)", node.Name, cpuUtil, memoryUtil),
				Impact:      "Consider consolidating workloads or downsizing",
				Savings:     co.calculateNodeCost(node.Name, "") * 24 * 30 * 0.7, // 70% potential savings
				Priority:    "medium",
				Timestamp:   time.Now(),
			})
		}

		// Over-provisioned node recommendation
		if cpuUtil > 90 || memoryUtil > 90 {
			recommendations = append(recommendations, Recommendation{
				Type:        "node_scaling",
				Resource:    node.Name,
				Description: fmt.Sprintf("Node %s is overutilized (CPU: %.1f%%, Memory: %.1f%%)", node.Name, cpuUtil, memoryUtil),
				Impact:      "Consider scaling up or adding more nodes",
				Savings:     -50, // Negative savings (cost increase but performance improvement)
				Priority:    "high",
				Timestamp:   time.Now(),
			})
		}
	}

	return recommendations
}

func (co *CostOptimizer) analyzePods(ctx context.Context) []Recommendation {
	recommendations := make([]Recommendation, 0)
	
	pods, err := co.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list pods: %v", err)
		return recommendations
	}

	podMetrics, err := co.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to get pod metrics: %v", err)
		return recommendations
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Find corresponding metrics
		var metrics *metricsv1beta1.PodMetrics
		for _, m := range podMetrics.Items {
			if m.Name == pod.Name && m.Namespace == pod.Namespace {
				metrics = &m
				break
			}
		}

		if metrics == nil {
			continue
		}

		// Analyze resource requests vs usage
		for i, container := range pod.Spec.Containers {
			if i >= len(metrics.Containers) {
				continue
			}

			containerMetrics := metrics.Containers[i]
			
			// Check CPU over-provisioning
			if container.Resources.Requests != nil {
				cpuRequest := container.Resources.Requests[corev1.ResourceCPU]
				cpuUsage := containerMetrics.Usage[corev1.ResourceCPU]
				
				if cpuRequest.MilliValue() > 0 && cpuUsage.MilliValue() < cpuRequest.MilliValue()/2 {
					recommendations = append(recommendations, Recommendation{
						Type:        "resource_rightsizing",
						Resource:    fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
						Namespace:   pod.Namespace,
						Description: fmt.Sprintf("Container %s is over-provisioned for CPU (request: %dm, usage: %dm)", container.Name, cpuRequest.MilliValue(), cpuUsage.MilliValue()),
						Impact:      "Reduce CPU request to optimize resource allocation",
						Savings:     15.0, // Estimated monthly savings
						Priority:    "low",
						Timestamp:   time.Now(),
					})
				}
			}

			// Check memory over-provisioning
			if container.Resources.Requests != nil {
				memRequest := container.Resources.Requests[corev1.ResourceMemory]
				memUsage := containerMetrics.Usage[corev1.ResourceMemory]
				
				if memRequest.Value() > 0 && memUsage.Value() < memRequest.Value()/2 {
					recommendations = append(recommendations, Recommendation{
						Type:        "resource_rightsizing",
						Resource:    fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
						Namespace:   pod.Namespace,
						Description: fmt.Sprintf("Container %s is over-provisioned for memory (request: %s, usage: %s)", container.Name, memRequest.String(), memUsage.String()),
						Impact:      "Reduce memory request to optimize resource allocation",
						Savings:     10.0, // Estimated monthly savings
						Priority:    "low",
						Timestamp:   time.Now(),
					})
				}
			}
		}
	}

	return recommendations
}

func (co *CostOptimizer) analyzeDeployments(ctx context.Context) []Recommendation {
	recommendations := make([]Recommendation, 0)
	
	deployments, err := co.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list deployments: %v", err)
		return recommendations
	}

	for _, deployment := range deployments.Items {
		// Check for low replica utilization during off-hours
		if deployment.Status.Replicas > 1 {
			recommendations = append(recommendations, Recommendation{
				Type:        "horizontal_scaling",
				Resource:    fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name),
				Namespace:   deployment.Namespace,
				Description: fmt.Sprintf("Deployment %s could benefit from auto-scaling based on metrics", deployment.Name),
				Impact:      "Implement HPA to scale based on CPU/memory usage",
				Savings:     25.0, // Estimated monthly savings
				Priority:    "medium",
				Timestamp:   time.Now(),
			})
		}

		// Check for missing resource requests/limits
		hasResources := false
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Resources.Requests != nil || container.Resources.Limits != nil {
				hasResources = true
				break
			}
		}

		if !hasResources {
			recommendations = append(recommendations, Recommendation{
				Type:        "resource_governance",
				Resource:    fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name),
				Namespace:   deployment.Namespace,
				Description: fmt.Sprintf("Deployment %s lacks resource requests/limits", deployment.Name),
				Impact:      "Add resource requests and limits for better scheduling and cost control",
				Savings:     20.0, // Estimated monthly savings through better resource management
				Priority:    "medium",
				Timestamp:   time.Now(),
			})
		}
	}

	return recommendations
}

func (co *CostOptimizer) calculateNodeCost(nodeName, instanceType string) float64 {
	if instanceType == "" {
		// Try to extract instance type from node name or use default
		for nodeType, cost := range co.costCalculator.NodeCostPerHour {
			if strings.Contains(nodeName, nodeType) {
				return cost
			}
		}
		return co.costCalculator.NodeCostPerHour["default"]
	}
	
	if cost, exists := co.costCalculator.NodeCostPerHour[instanceType]; exists {
		return cost
	}
	return co.costCalculator.NodeCostPerHour["default"]
}

// HTTP Handlers
func (co *CostOptimizer) handleNodeMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	nodeMetrics := co.getNodeMetrics(ctx)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodeMetrics)
}

func (co *CostOptimizer) handlePodMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	podMetrics := co.getPodMetrics(ctx)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(podMetrics)
}

func (co *CostOptimizer) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(co.recommendations)
}

func (co *CostOptimizer) handleCostSummary(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	summary := co.generateCostSummary(ctx)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (co *CostOptimizer) handleOptimize(w http.ResponseWriter, r *http.Request) {
	// Trigger immediate analysis
	go co.analyzeAndGenerateRecommendations()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "optimization_triggered",
		"message": "Cost analysis has been triggered",
	})
}

func (co *CostOptimizer) handleActions(w http.ResponseWriter, r *http.Request) {
	// Return available optimization actions
	actions := []OptimizationAction{
		{
			ID:        "1",
			Type:      "scale_down",
			Resource:  "default/nginx-deployment",
			Namespace: "default",
			Action:    "Scale deployment to 1 replica",
			Parameters: map[string]interface{}{
				"replicas": 1,
			},
			Status:    "pending",
			CreatedAt: time.Now(),
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(actions)
}

func (co *CostOptimizer) handleExecuteAction(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	actionID := vars["id"]
	
	// In a real implementation, this would execute the optimization action
	log.Printf("Executing optimization action: %s", actionID)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "executed",
		"action_id": actionID,
		"message": "Optimization action executed successfully",
	})
}

func (co *CostOptimizer) getNodeMetrics(ctx context.Context) []NodeMetrics {
	metrics := make([]NodeMetrics, 0)
	
	nodes, err := co.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list nodes: %v", err)
		return metrics
	}

	nodeMetricsList, err := co.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to get node metrics: %v", err)
		return metrics
	}

	for _, node := range nodes.Items {
		var nodeMetrics *metricsv1beta1.NodeMetrics
		for _, m := range nodeMetricsList.Items {
			if m.Name == node.Name {
				nodeMetrics = &m
				break
			}
		}

		if nodeMetrics == nil {
			continue
		}

		cpuCapacity := node.Status.Capacity[corev1.ResourceCPU]
		memoryCapacity := node.Status.Capacity[corev1.ResourceMemory]
		cpuUsage := nodeMetrics.Usage[corev1.ResourceCPU]
		memoryUsage := nodeMetrics.Usage[corev1.ResourceMemory]

		cpuUtil := float64(cpuUsage.MilliValue()) / float64(cpuCapacity.MilliValue()) * 100
		memoryUtil := float64(memoryUsage.Value()) / float64(memoryCapacity.Value()) * 100

		instanceType := co.extractInstanceType(node.Name)
		hourlyCost := co.calculateNodeCost(node.Name, instanceType)

		metrics = append(metrics, NodeMetrics{
			Name:              node.Name,
			CPUUsage:          float64(cpuUsage.MilliValue()) / 1000,
			MemoryUsage:       float64(memoryUsage.Value()) / (1024 * 1024 * 1024),
			CPUCapacity:       float64(cpuCapacity.MilliValue()) / 1000,
			MemoryCapacity:    float64(memoryCapacity.Value()) / (1024 * 1024 * 1024),
			CPUUtilization:    cpuUtil,
			MemoryUtilization: memoryUtil,
			EstimatedCost:     hourlyCost * 24 * 30, // Monthly cost
			InstanceType:      instanceType,
		})
	}

	return metrics
}

func (co *CostOptimizer) getPodMetrics(ctx context.Context) []PodMetrics {
	metrics := make([]PodMetrics, 0)
	
	pods, err := co.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list pods: %v", err)
		return metrics
	}

	podMetricsList, err := co.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to get pod metrics: %v", err)
		return metrics
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		var podMetrics *metricsv1beta1.PodMetrics
		for _, m := range podMetricsList.Items {
			if m.Name == pod.Name && m.Namespace == pod.Namespace {
				podMetrics = &m
				break
			}
		}

		if podMetrics == nil {
			continue
		}

		// Calculate total pod resource usage
		var totalCPUUsage, totalMemUsage resource.Quantity
		var totalCPURequest, totalMemRequest, totalCPULimit, totalMemLimit resource.Quantity

		for _, containerMetrics := range podMetrics.Containers {
			cpuUsage := containerMetrics.Usage[corev1.ResourceCPU]
			memUsage := containerMetrics.Usage[corev1.ResourceMemory]
			totalCPUUsage.Add(cpuUsage)
			totalMemUsage.Add(memUsage)
		}

		for _, container := range pod.Spec.Containers {
			if container.Resources.Requests != nil {
				if cpuReq := container.Resources.Requests[corev1.ResourceCPU]; !cpuReq.IsZero() {
					totalCPURequest.Add(cpuReq)
				}
				if memReq := container.Resources.Requests[corev1.ResourceMemory]; !memReq.IsZero() {
					totalMemRequest.Add(memReq)
				}
			}
			if container.Resources.Limits != nil {
				if cpuLimit := container.Resources.Limits[corev1.ResourceCPU]; !cpuLimit.IsZero() {
					totalCPULimit.Add(cpuLimit)
				}
				if memLimit := container.Resources.Limits[corev1.ResourceMemory]; !memLimit.IsZero() {
					totalMemLimit.Add(memLimit)
				}
			}
		}

		// Estimate pod cost based on resource requests
		estimatedCost := co.estimatePodCost(totalCPURequest, totalMemRequest)

		metrics = append(metrics, PodMetrics{
			Name:          pod.Name,
			Namespace:     pod.Namespace,
			CPUUsage:      float64(totalCPUUsage.MilliValue()) / 1000,
			MemoryUsage:   float64(totalMemUsage.Value()) / (1024 * 1024 * 1024),
			CPURequest:    float64(totalCPURequest.MilliValue()) / 1000,
			MemoryRequest: float64(totalMemRequest.Value()) / (1024 * 1024 * 1024),
			CPULimit:      float64(totalCPULimit.MilliValue()) / 1000,
			MemoryLimit:   float64(totalMemLimit.Value()) / (1024 * 1024 * 1024),
			EstimatedCost: estimatedCost,
		})
	}

	return metrics
}

func (co *CostOptimizer) generateCostSummary(ctx context.Context) ClusterCostSummary {
	nodeMetrics := co.getNodeMetrics(ctx)
	podMetrics := co.getPodMetrics(ctx)
	
	var totalComputeCost, totalStorageCost, wastedResources float64
	namespaceCosts := make(map[string]float64)
	
	// Calculate compute costs
	for _, node := range nodeMetrics {
		totalComputeCost += node.EstimatedCost
		
		// Calculate wasted resources (underutilized capacity)
		if node.CPUUtilization < 50 || node.MemoryUtilization < 50 {
			wastedResources += node.EstimatedCost * 0.3 // 30% waste factor
		}
	}
	
	// Calculate namespace costs
	for _, pod := range podMetrics {
		namespaceCosts[pod.Namespace] += pod.EstimatedCost
	}
	
	// Estimate storage costs (simplified)
	totalStorageCost = 100.0 // Placeholder
	
	// Calculate potential savings from recommendations
	var potentialSavings float64
	for _, rec := range co.recommendations {
		potentialSavings += rec.Savings
	}
	
	return ClusterCostSummary{
		TotalMonthlyCost:     totalComputeCost + totalStorageCost,
		ComputeCost:          totalComputeCost,
		StorageCost:          totalStorageCost,
		WastedResources:      wastedResources,
		PotentialSavings:     potentialSavings,
		NodeCount:            len(nodeMetrics),
		PodCount:             len(podMetrics),
		NamespaceCosts:       namespaceCosts,
		RecommendationCount:  len(co.recommendations),
		LastUpdated:          time.Now(),
	}
}

func (co *CostOptimizer) extractInstanceType(nodeName string) string {
	// Simple heuristic to extract instance type from node name
	for instanceType := range co.costCalculator.NodeCostPerHour {
		if strings.Contains(strings.ToLower(nodeName), instanceType) {
			return instanceType
		}
	}
	return "default"
}

func (co *CostOptimizer) estimatePodCost(cpuRequest, memRequest resource.Quantity) float64 {
	// Simple cost estimation based on resource requests
	// This is a simplified calculation - in reality, you'd want more sophisticated cost allocation
	cpuCost := float64(cpuRequest.MilliValue()) / 1000 * 0.05 * 24 * 30 // $0.05 per CPU hour
	memCost := float64(memRequest.Value()) / (1024 * 1024 * 1024) * 0.01 * 24 * 30 // $0.01 per GB hour
	return cpuCost + memCost
}