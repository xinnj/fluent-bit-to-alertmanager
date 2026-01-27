package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	clientruntime "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/models"
)

type LogEntry map[string]any

var alertManagerHost, alertManagerBasePath, alertManagerScheme string
var alertManagerClient alert.ClientService

func initLogging() {
	// 将日志输出到 stdout，设置可读的时间/文件信息
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

func initAlertManagerClient() {
	cr := clientruntime.New(alertManagerHost, alertManagerBasePath, []string{alertManagerScheme})
	alertManagerClient = alert.New(cr, strfmt.Default)
}

func parseLogs(r *http.Request) ([]LogEntry, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body: %v", err)
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Printf("Warning: failed to close request body: %v", err)
		}
	}()

	var logs []LogEntry
	err = json.Unmarshal(body, &logs)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling logs: %v", err)
	}

	return logs, nil
}

func createAlertFromLog(oneLog LogEntry) models.PostableAlerts {
	getString := func(key string) string {
		if val, ok := oneLog[key].(string); ok {
			return val
		}
		return ""
	}

	var logTime strfmt.DateTime
	timeStr := getString("time")
	if timeStr == "" {
		logTime = strfmt.DateTime(time.Now())
	} else {
		var err error
		logTime, err = strfmt.ParseDateTime(timeStr)
		if err != nil {
			logTime = strfmt.DateTime(time.Now())
		}
	}

	postableAlert := models.PostableAlert{
		StartsAt: logTime,
		Annotations: map[string]string{
			"summary":     "ERROR in log",
			"description": "Log level with ERROR in log",
		},
		Alert: models.Alert{
			Labels: map[string]string{
				"severity":  "warning",
				"alertname": "ErrorLog",
				"time":      logTime.String(),
				"log":       getString("log"),
				"namespace": getString("namespace"),
				"app":       getString("app"),
			},
		},
	}
	alerts := models.PostableAlerts{&postableAlert}

	return alerts
}

func sendAlerts(alerts models.PostableAlerts) error {
	response, err := alertManagerClient.PostAlerts(alert.NewPostAlertsParams().WithAlerts(alerts))
	if err != nil {
		return fmt.Errorf("error posting alerts: %v", err)
	}

	if response != nil && response.IsSuccess() {
		log.Printf("Alerts posted successfully.")
	} else {
		log.Printf("PostAlerts response: %#v", response)
	}

	return nil
}

func receiveLog(w http.ResponseWriter, r *http.Request) {
	logs, err := parseLogs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// As the log length is long, let's send alert one by one
	for _, oneLog := range logs {
		log.Printf("Log: %v\n", oneLog)

		alerts := createAlertFromLog(oneLog)

		err = sendAlerts(alerts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func parseAlertManagerURL() {
	alertManagerURL := os.Getenv("ALERTMANAGER_URL")
	if alertManagerURL == "" {
		defaultURL := "http://localhost:9093"
		log.Printf("ALERTMANAGER_URL not set, using default %s", defaultURL)
		alertManagerURL = defaultURL
	}

	u, err := url.Parse(alertManagerURL)
	if err != nil {
		log.Fatalf("Can't parse ALERTMANAGER_URL '%s': %v", alertManagerURL, err)
	}

	if u.Host == "" {
		log.Printf("Parsed ALERTMANAGER_URL has empty host, using default host 'localhost:9093'")
		u = &url.URL{Scheme: "http", Host: "localhost:9093", Path: ""}
	}

	alertManagerHost = u.Host
	alertManagerBasePath = u.Path
	alertManagerScheme = u.Scheme

	log.Printf("Alertmanager configured -> scheme: %s host: %s basePath: %s", alertManagerScheme, alertManagerHost, alertManagerBasePath)
}

func logRequest(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("REQ %s - %s %s", r.RemoteAddr, r.Method, r.URL.RequestURI())
		h(w, r)
	}
}

func main() {
	initLogging()
	parseAlertManagerURL()
	initAlertManagerClient()

	http.HandleFunc("/", logRequest(receiveLog))
	http.HandleFunc("/health", healthCheck)

	log.Print("Listening on port 8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
