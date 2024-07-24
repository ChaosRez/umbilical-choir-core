package controller

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type PrometheusClient struct {
	Client         api.Client
	API            v1.API
	PushGatewayURL string
}

// NewPrometheusClient creates a new Prometheus API client
func NewPrometheusClient(promHost string) (*PrometheusClient, error) {
	client, err := api.NewClient(api.Config{
		Address: fmt.Sprintf("http://%s:%s", promHost, "9092"),
	})
	if err != nil {
		log.Errorf("Error creating API client for Prometheus: %v", err)
		return nil, err
	}

	prometheusV1 := v1.NewAPI(client)
	log.Info("Created Prometheus API client")

	return &PrometheusClient{
		Client:         client,
		API:            prometheusV1,
		PushGatewayURL: fmt.Sprintf("http://%s:%s", promHost, "9091"),
	}, nil
}

func (p *PrometheusClient) Query(query string) (model.Value, error) {
	// ctx (context) is usually passed around between functions and methods so that you can cancel long-running operations,
	// set deadlines, or pass request-scoped data. It's not specific to the Prometheus client
	ctx := context.Background()
	result, _, err := p.API.Query(ctx, query, time.Now())
	return result, err
}

func (p *PrometheusClient) PushGatewayDeleteJob(job string) error {
	// Clean up Pushgateway
	err := push.New(p.PushGatewayURL, job).Delete()
	if err != nil {
		return err
	}
	log.Infof("Deleted job '%s' from Pushgateway", job)
	return nil
}

func (p *PrometheusClient) PushGatewayDeleteMetricsForProgram(job string, program string) error {
	url := fmt.Sprintf("%s/metrics/job/%s/program/%s", p.PushGatewayURL, job, program)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to delete metrics for program '%s': %s", program, resp.Status)
	}

	log.Infof("Deleted metrics for program='%s'", program)
	return nil
}
