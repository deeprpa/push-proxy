package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/nettools"
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
	labels          map[string]string
	pushInterval    time.Duration

	autoCleanup = true

	pushURL string
)

func main() {

	rootCmd.Flags().StringVarP(&targetAddr, "target-addr", "t", "http://localhost:9090/metrics", "The address of the target to scrape metrics from")
	rootCmd.Flags().StringVar(&pushgatewayAddr, "pushgateway-addr", "http://localhost:9091", "The address of the Pushgateway to push metrics to")
	rootCmd.Flags().StringVar(&pushgatewayUser, "pushgateway-user", "", "The username for Pushgateway basic auth (if required)")
	rootCmd.Flags().StringVar(&pushgatewayPass, "pushgateway-pass", "", "The password for Pushgateway basic auth (if required)")
	rootCmd.Flags().StringVarP(&pushJobName, "label-job", "j", "", "The job name to use when pushing metrics to the Pushgateway")
	rootCmd.Flags().StringVarP(&instanceLabel, "label-instance", "n", getDefaultInstanceLabel(), "The instance label to use when pushing metrics to the Pushgateway")
	rootCmd.Flags().StringVar(&namespaceLabel, "label-namespace", "", "The namespace label to use when pushing metrics to the Pushgateway")
	rootCmd.Flags().StringToStringVar(&labels, "labels", map[string]string{}, "Additional labels to add when pushing metrics to the Pushgateway, in key=value format")
	rootCmd.Flags().DurationVarP(&pushInterval, "interval", "i", 15*time.Second, "The interval at which to push metrics to the Pushgateway")
	rootCmd.Flags().BoolVar(&autoCleanup, "auto-cleanup", true, "Automatically delete metrics from Pushgateway on shutdown")

	rootCmd.Execute()
}

func mainCmd(cmd *cobra.Command, args []string) {
	ticker := time.NewTicker(pushInterval)
	defer ticker.Stop()
	if pushJobName == "" {
		logs.Errorf("Push job name is empty, it's recommended to set a job name using --label-job")
		return
	}
	if instanceLabel == "" {
		logs.Errorf("Instance label is empty, it's recommended to set an instance label using --label-instance")
		return
	}
	logs.Infof("Starting push-proxy with target=%s, pushgateway=%s, job=%s, instance=%s, interval=%s", targetAddr, pushgatewayAddr, pushJobName, instanceLabel, pushInterval)
	for k, v := range labels {
		logs.Infof("Additional label: %s=%s", k, v)
	}
	pushgatewayAddr = strings.TrimRight(pushgatewayAddr, "/")

	ctx := lifecycle.Std().Context()
	go func() {
		for {
			select {
			case <-ctx.Done():
				logs.Infof("Shutting down push-proxy")
				return
			case <-ticker.C:
				pushOnce(ctx)
			}
		}
	}()
	if autoCleanup {
		lifecycle.Std().AddCloseFunc(cleanupGatewayInstance)
	}
	lifecycle.Std().WaitExit()
}

func cleanupGatewayInstance() error {
	if !autoCleanup {
		return nil
	}
	pushURL := getPushURL()
	req, err := http.NewRequest("DELETE", pushURL, nil)
	if err != nil {
		return fmt.Errorf("error creating cleanup request to Pushgateway: %v", err)
	}
	if pushgatewayUser != "" && pushgatewayPass != "" {
		req.SetBasicAuth(pushgatewayUser, pushgatewayPass)
	}
	logs.Info(pushURL)
	for k, v := range req.Header {
		logs.Infof("Header %s: %v", k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error during cleanup request to Pushgateway: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logs.Infof("Cleanup successful for job=%s, instance=%s", pushJobName, instanceLabel)
	} else {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Pushgateway cleanup error: %s, body: %s", resp.Status, body)
	}
	return nil
}

func getPushURL() string {
	if pushURL != "" {
		return pushURL
	}
	pushURL = fmt.Sprintf("%s/metrics/job/%s/instance/%s", pushgatewayAddr, pushJobName, instanceLabel)
	for k, v := range labels {
		pushURL += fmt.Sprintf("/%s/%s", k, v)
	}
	return pushURL
}

func pushOnce(ctx context.Context) {
	metrics, err := fetchMetrics(ctx, targetAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch metrics: %v\n", err)
		return
	}

	// pushURL := fmt.Sprintf("%s/metrics/job/%s/instance/%s", pushgatewayAddr, pushJobName, instanceLabel)
	pushURL := getPushURL()

	req, err := http.NewRequestWithContext(ctx, "POST", pushURL, metrics)
	if err != nil {
		logs.Errorf("Error creating request to Pushgateway: %v", err)
		return
	}
	if pushgatewayUser != "" && pushgatewayPass != "" {
		req.SetBasicAuth(pushgatewayUser, pushgatewayPass)
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logs.Errorf("Error pushing metrics to Pushgateway: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logs.Infof("[%s] Metrics pushed successfully", time.Now().Format(time.RFC3339))
	} else {
		body, _ := io.ReadAll(resp.Body)
		logs.Errorf("Pushgateway error: %s, %s body: %s", resp.Status, pushURL, body)
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

func getDefaultInstanceLabel() string {
	ins := os.Getenv("POD_NAME")
	if ins != "" {
		return ins
	}
	ins = os.Getenv("POD_IP")
	if ins != "" {
		return ins
	}
	ip := nettools.MustLocalPrimearyIP()
	return ip.String()
}
