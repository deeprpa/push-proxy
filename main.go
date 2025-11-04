package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
)

var (
	rootCmd = cobra.Command{
		Use:   "push-proxy",
		Short: "Business indicator push agent, manage indicator life cycle, and forward metrics to Pushgateway. Retry",
		Run:   mainCmd,
	}

	targetAddr      string
	pushgatewayAddr string
	pushgatewayUser string
	pushgatewayPass string
	pushJobName     string
	instanceLabel   string
	namespaceLabel  string
	pushInterval    time.Duration
)

func main() {
	rootCmd.Flags().StringVar(&targetAddr, "target-addr", "http://localhost:9090/metrics", "The address of the target to scrape metrics from")
	rootCmd.Flags().StringVar(&pushgatewayAddr, "pushgateway-addr", "http://localhost:9091", "The address of the Pushgateway to push metrics to")
	rootCmd.Flags().StringVar(&pushgatewayUser, "pushgateway-user", "", "The username for Pushgateway basic auth (if required)")
	rootCmd.Flags().StringVar(&pushgatewayPass, "pushgateway-pass", "", "The password for Pushgateway basic auth (if required)")
	rootCmd.Flags().StringVar(&pushJobName, "job-name", "job", "The job name to use when pushing metrics to the Pushgateway")
	rootCmd.Flags().StringVar(&instanceLabel, "label-instance", "none", "The instance label to use when pushing metrics to the Pushgateway")
	rootCmd.Flags().StringVar(&namespaceLabel, "label-namespace", "default", "The namespace label to use when pushing metrics to the Pushgateway")
	rootCmd.Flags().DurationVar(&pushInterval, "interval", 30*time.Second, "The interval at which to push metrics to the Pushgateway")

	rootCmd.Execute()
}

func mainCmd(cmd *cobra.Command, args []string) {
	ticker := time.NewTicker(pushInterval)
	defer ticker.Stop()

	ctx := lifecycle.Std().Context()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Sidecar stopped")
			return
		case <-ticker.C:
			pushOnce(ctx)
		}
	}
}

func pushOnce(ctx context.Context) {
	metrics, err := fetchMetrics(ctx, targetAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch metrics: %v\n", err)
		return
	}

	pushUrl := fmt.Sprintf("%s/metrics/job/%s/instance/%s", pushgatewayAddr, pushJobName, instanceLabel)
	// Optionally include namespace label if provided
	if namespaceLabel != "" {
		pushUrl = fmt.Sprintf("%s/namespace/%s", pushUrl, namespaceLabel)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pushUrl, metrics)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating push request: %v\n", err)
		return
	}
	if pushgatewayUser != "" && pushgatewayPass != "" {
		req.SetBasicAuth(pushgatewayUser, pushgatewayPass)
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Push to Pushgateway failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logs.Infof("[%s] Metrics pushed successfully", time.Now().Format(time.RFC3339))
	} else {
		body, _ := io.ReadAll(resp.Body)
		logs.Errorf("Pushgateway error: %s, body: %s", resp.Status, body)
	}
}

func fetchMetrics(ctx context.Context, url string) (io.Reader, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("metrics endpoint returned %v: %s", resp.Status, body)
	}
	return resp.Body, nil
}
