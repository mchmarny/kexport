package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	meta "cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

const (
	notSetValue = "none"
)

var (
	logger         = log.New(os.Stdout, "[ke] ", 0)
	projectID      = mustEnvVar("PROJECT", notSetValue)
	datasetName    = mustEnvVar("DATASET", "kadvice")
	tableName      = mustEnvVar("TABLE", "metrics")
	intervalPeriod = mustEnvVar("INTERVAL", "60s")
	numGoroutines  = 10

	// TODO: externalize this
	excludeNS = []string{
		"istio-system",
		"kube-system",
	}
)

func main() {

	flag.Parse()
	ctx := context.Background()
	parseProject()

	// Create the BigQuery client.
	bq, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		logger.Fatal(err)
	}

	dataset := bq.DatasetInProject(projectID, datasetName)
	if err := createDataset(ctx, dataset); err != nil {
		logger.Fatalf("Error creating dataset %s/%s: %v", projectID, datasetName, err)
	}

	table := dataset.Table(tableName)
	if err := createTable(ctx, table); err != nil {
		logger.Fatalf("Error creating table %s/%s.%s: %v", projectID, datasetName, tableName, err)
	}

	clusters := getAllClusters(ctx)

	// Start all background goroutines.
	ch := make(chan *Cluster, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go backgroundThread(ctx, table, ch)
	}

	// Loop forever, pushing each cluster to the background workers for processing, then sleep.
	for {
		for _, c := range clusters {
			ch <- c
		}
		d, err := time.ParseDuration(intervalPeriod)
		if err != nil {
			time.Sleep(60 * time.Second)
		} else {
			time.Sleep(d)
		}
	}
}

func mustEnvVar(key, fallbackValue string) string {
	if val, ok := os.LookupEnv(key); ok {
		logger.Printf("%s: %s", key, val)
		return strings.TrimSpace(val)
	}

	if fallbackValue == "" {
		logger.Fatalf("Required envvar not set: %s", key)
	}

	logger.Printf("%s: %s (not set, using default)", key, fallbackValue)
	return fallbackValue
}

func parseProject() {
	if projectID != notSetValue {
		return
	}

	mc := meta.NewClient(&http.Client{Transport: userAgentTransport{
		userAgent: "kexport",
		base:      http.DefaultTransport,
	}})
	p, err := mc.ProjectID()
	if err != nil {
		logger.Fatalf("Error creating metadata client: %v", err)
	}
	projectID = p
}

func createDataset(ctx context.Context, dataset *bigquery.Dataset) error {
	err := dataset.Create(ctx, nil)
	if err == nil {
		logger.Println("Created dataset")
		return nil
	}
	if e, ok := err.(*googleapi.Error); ok {
		if e.Code == 409 {
			// Dataset already exists. This is fine.
			return nil
		}
	}
	return err
}

func createTable(ctx context.Context, table *bigquery.Table) error {
	schema, err := bigquery.InferSchema(Usage{})
	if err != nil {
		return fmt.Errorf("unable to infer schema")
	}
	err = table.Create(ctx, &bigquery.TableMetadata{Schema: schema})
	if err == nil {
		logger.Println("Created table")
		return nil
	}
	if e, ok := err.(*googleapi.Error); ok {
		if e.Code == 409 {
			// Table already exists. This is fine.
			return nil
		}
	}
	return err
}

func backgroundThread(ctx context.Context, table *bigquery.Table, ch <-chan *Cluster) {
	for {
		c, ok := <-ch
		if !ok {
			return
		}
		st := time.Now()

		usage := make(map[string]*Usage)

		// Fetch pods.
		pods, err := c.clientset.CoreV1().Pods("").List(metav1.ListOptions{})
		if err != nil {
			logger.Printf("Error fetching pods for %s/%s: %v", c.project, c.cluster, err)
			if errors.IsUnauthorized(err) {
				logger.Printf("Authorization has expired, exiting")
				os.Exit(0)
			}
			continue
		}

		for _, pod := range pods.Items {

			if skip(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name) {
				continue
			}

			u := getPod(usage, c.project, c.cluster, pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
			u.ServiceAccount = pod.Spec.ServiceAccountName

			for _, container := range pod.Spec.Containers {
				if value, ok := container.Resources.Requests["cpu"]; ok {
					u.ReservedCPU += value.MilliValue()
				}
				if value, ok := container.Resources.Requests["memory"]; ok {
					u.ReservedRAM += value.Value()
				}
			}
		}

		// Fetch current usage from metrics-server.
		metrics := &PodMetricsList{}
		res := c.clientset.RESTClient().Get().Prefix("/apis/metrics.k8s.io/v1beta1/pods").Do()
		obj, err := res.Raw()
		if err != nil {
			logger.Printf("Error fetching metrics for %s/%s: %v", c.project, c.cluster, err)
			continue
		}
		if err := json.Unmarshal(obj, metrics); err != nil {
			logger.Printf("Error fetching metrics for %s/%s: %v", c.project, c.cluster, err)
			continue
		}
		for _, item := range metrics.Items {

			if skip(item.Metadata.Namespace, item.Metadata.Name) {
				continue
			}

			u := getPod(usage, c.project, c.cluster, item.Metadata.Namespace, item.Metadata.Name)
			for _, container := range item.Containers {
				m, err := resource.ParseQuantity(container.Usage.Memory)
				if err != nil {
					logger.Printf("Error parsing memory %q: %v", container.Usage.Memory, err)
				} else {
					u.UsedRAM += m.Value()
				}

				c, err := resource.ParseQuantity(container.Usage.CPU)
				if err != nil {
					logger.Printf("Error parsing CPU %q: %v", container.Usage.CPU, err)
				} else {
					u.UsedCPU += c.MilliValue()
				}
			}
		}

		// Write rows to BigQuery.
		var rows []*Usage
		for _, row := range usage {
			if skip(row.Namespace, row.Pod) {
				continue
			}
			rows = append(rows, row)
		}

		if len(rows) == 0 {
			continue
		}

		u := table.Uploader()
		if err := u.Put(ctx, rows); err != nil {
			logger.Printf("Error writing to BigQuery: %v", err)
			continue
		}

		logger.Printf("Sent %d rows to bigquery for project %q cluster %q in %s",
			len(rows), c.project, c.cluster, time.Since(st))
	}
}

func getPod(usage map[string]*Usage, project, cluster, namespace, pod string) *Usage {
	key := fmt.Sprintf("%s/%s/%s/%s", project, cluster, namespace, pod)
	if u, ok := usage[key]; ok {
		return u
	}
	u := &Usage{
		Timestamp: time.Now(),
		Project:   project,
		Cluster:   cluster,
		Namespace: namespace,
		Pod:       pod,
	}
	usage[key] = u
	return u
}

func getClientset(ctx context.Context, cluster *container.Cluster, token string) (*kubernetes.Clientset, error) {
	if cluster.Status != "RUNNING" {
		return nil, fmt.Errorf("cluster is not running")
	}

	ca, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, fmt.Errorf("error decoding cluster masterAuth: %v", err)
	}

	// Configure TLS, with certificates if basic auth is enabled.
	tlsconfig := rest.TLSClientConfig{CAData: ca}
	if cluster.MasterAuth.Username != "" {
		cc, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClientCertificate)
		if err != nil {
			return nil, fmt.Errorf("error decoding cluster masterAuth: %v", err)
		}
		ck, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("error decoding cluster masterAuth: %v", err)
		}
		tlsconfig.CertData = cc
		tlsconfig.KeyData = ck
	}

	config := &rest.Config{
		Host:            fmt.Sprintf("https://%s/", cluster.Endpoint),
		TLSClientConfig: tlsconfig,
		Username:        cluster.MasterAuth.Username,
		Password:        cluster.MasterAuth.Password,
		BearerToken:     token,
	}
	if err := rest.SetKubernetesDefaults(config); err != nil {
		return nil, fmt.Errorf("error setting Kubernetes config: %v", err)
	}
	return kubernetes.NewForConfig(config)
}

func getAllClusters(ctx context.Context) []*Cluster {
	// Authenticate to the Google Cloud APIs.
	hc, err := google.DefaultClient(ctx, container.CloudPlatformScope)
	if err != nil {
		logger.Fatalf("Could not get authenticated client: %v", err)
	}
	gke, err := container.New(hc)
	if err != nil {
		logger.Fatalf("Could not initialize GKE client: %v", err)
	}

	// Get a token source suitable for querying GKE clusters.
	ts, err := google.DefaultTokenSource(ctx, container.CloudPlatformScope)
	if err != nil {
		logger.Fatalf("could not get Google token source: %v", err)
	}
	token, err := ts.Token()
	if err != nil {
		logger.Fatalf("could not get Google token: %v", err)
	}

	logger.Printf("Fetching a list of all clusters")
	var ret []*Cluster
	clusters, err := gke.Projects.Zones.Clusters.List(projectID, "-").Do()
	if err != nil {
		logger.Fatalf("Could not get the list of GKE clusters: %v", err)
	}
	for _, cluster := range clusters.Clusters {
		clientset, err := getClientset(ctx, cluster, token.AccessToken)
		if err != nil {
			logger.Printf("Error getting clientset for %s/%s: %v", projectID, cluster.Name, err)
			continue
		}
		ret = append(ret, &Cluster{
			project:   projectID,
			cluster:   cluster.Name,
			clientset: clientset,
		})
		logger.Printf("  %s/%s", projectID, cluster.Name)
	}

	return ret
}

func skip(ns, pod string) bool {
	for _, n := range excludeNS {
		if ns == n {
			logger.Printf("Skipping NS: %s", ns)
			return true
		}
	}

	// skip self
	if ns == "default" && pod == "kexport" {
		logger.Printf("Skipping Self: %s", pod)
		return true
	}

	return false
}

// GCP Metadata
// https://godoc.org/cloud.google.com/go/compute/metadata#example-NewClient
type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}

type PodMetricsList struct {
	Kind       string
	APIVersion string
	Items      []struct {
		Timestamp string
		Window    string
		Metadata  struct {
			Namespace string
			Name      string
			SelfLink  string
		}
		Containers []struct {
			Name  string
			Usage struct {
				CPU    string
				Memory string
			}
		}
	}
}

// Cluster holds cluster data
type Cluster struct {
	project   string
	cluster   string
	clientset *kubernetes.Clientset
}

// Usage represents the metric written to BQ
type Usage struct {
	Timestamp      time.Time `bigquery:"metric_time"`
	Project        string    `bigquery:"project"`
	Cluster        string    `bigquery:"cluster"`
	Namespace      string    `bigquery:"namespace"`
	ServiceAccount string    `bigquery:"serviceaccount"`
	Pod            string    `bigquery:"pod"`
	ReservedCPU    int64     `bigquery:"reserved_cpu"`
	ReservedRAM    int64     `bigquery:"reserved_ram"`
	UsedCPU        int64     `bigquery:"used_cpu"`
	UsedRAM        int64     `bigquery:"used_ram"`
}
