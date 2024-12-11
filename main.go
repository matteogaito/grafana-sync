package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
)

var (
	apiKey    string
	baseURL   string
	directory string
	action    string
	folder    string
)

func init() {
	flag.StringVar(&apiKey, "apikey", "", "Grafana API key")
	flag.StringVar(&baseURL, "url", "", "Grafana base URL")
	flag.StringVar(&directory, "directory", "grafana_data", "Directory to store/load Grafana data")
	flag.StringVar(&action, "action", "pull", "Action to perform: pull or push")
	flag.StringVar(&folder, "folder", "", "Specify a folder for pulling dashboards (optional)")
}

func main() {
	flag.Parse()

	if apiKey == "" || baseURL == "" {
		fmt.Println("Error: apikey and url are required")
		os.Exit(1)
	}

	switch action {
	case "pull-dashboards":
		pullDashboards()
	case "pull-datasources":
		pullDatasources()
	case "pull-folders":
		pullFolders()
	case "pull-notifications":
		pullNotificationChannels()
	case "push-dashboards":
		pushDashboards()
	case "push-datasources":
		pushDatasources()
	case "push-folders":
		pushFolders()
	case "push-notifications":
		pushNotificationChannels()
	case "pull":
		pullData()
	case "push":
		pushData()
	default:
		fmt.Println("Error: action must be one of 'pull', 'push', 'pull-dashboards', 'pull-datasources', 'pull-folders', 'pull-notifications', 'push-dashboards', 'push-datasources', 'push-folders', 'push-notifications'")
		os.Exit(1)
	}
}

func getFolderID(folderName string) string {
	fmt.Printf("Fetching folder ID for: %s\n", folderName)
	url := fmt.Sprintf("%s/api/folders", baseURL)
	data := sendRequest("GET", url, nil)

	var folders []map[string]interface{}
	err := json.Unmarshal(data, &folders)
	if err != nil {
		fmt.Println("Error unmarshalling folders:", err)
		os.Exit(1)
	}

	for _, folder := range folders {
		if folder["title"] == folderName {
			return fmt.Sprintf("%v", folder["id"])
		}
	}

	fmt.Printf("Error: folder '%s' not found\n", folderName)
	os.Exit(1)
	return ""
}

// Pull all data from Grafana
func pullData() {
	pullDashboards()
	pullDatasources()
	pullFolders()
	pullNotificationChannels()
}

// Push all data to Grafana
func pushData() {
	pushDashboards()
	pushDatasources()
	pushFolders()
	pushNotificationChannels()
}

// Pull Functions

func pullDashboards() {
	fmt.Println("Pulling dashboards...")
	url := fmt.Sprintf("%s/api/search?type=dash-db", baseURL)
	if folder != "" {
		url += fmt.Sprintf("&folderIds=%s", getFolderID(folder))
	}
	data := sendRequest("GET", url, nil)
	var dashboards []map[string]interface{}
	err := json.Unmarshal(data, &dashboards)
	if err != nil {
		fmt.Println("Error unmarshalling dashboards:", err)
		return
	}

	dashboardDir := filepath.Join(directory, "dashboards")
	os.MkdirAll(dashboardDir, os.ModePerm)

	for _, db := range dashboards {
		uid := db["uid"].(string)
		title := db["title"].(string)
		dashboardData := downloadDashboard(uid)
		err := saveToFile(filepath.Join(dashboardDir, title+".json"), dashboardData)
		if err != nil {
			fmt.Printf("Error saving dashboard %s: %v\n", title, err)
			continue
		}
		fmt.Printf("Saved dashboard: %s\n", title)
	}
}

func pullDatasources() {
	fmt.Println("Pulling datasources...")
	url := fmt.Sprintf("%s/api/datasources", baseURL)
	data := sendRequest("GET", url, nil)

	datasourceDir := filepath.Join(directory, "datasources")
	os.MkdirAll(datasourceDir, os.ModePerm)
	err := saveToFile(filepath.Join(datasourceDir, "datasources.json"), data)
	if err != nil {
		fmt.Println("Error saving datasources:", err)
		return
	}
	fmt.Println("Saved datasources")
}

func pullFolders() {
	fmt.Println("Pulling folders...")
	url := fmt.Sprintf("%s/api/folders", baseURL)
	data := sendRequest("GET", url, nil)

	folderDir := filepath.Join(directory, "folders")
	os.MkdirAll(folderDir, os.ModePerm)
	err := saveToFile(filepath.Join(folderDir, "folders.json"), data)
	if err != nil {
		fmt.Println("Error saving folders:", err)
		return
	}
	fmt.Println("Saved folders")
}

func pullNotificationChannels() {
	fmt.Println("Pulling notification channels...")
	url := fmt.Sprintf("%s/api/alert-notifications", baseURL)
	data := sendRequest("GET", url, nil)

	notificationDir := filepath.Join(directory, "notifications")
	os.MkdirAll(notificationDir, os.ModePerm)
	err := saveToFile(filepath.Join(notificationDir, "notifications.json"), data)
	if err != nil {
		fmt.Println("Error saving notification channels:", err)
		return
	}
	fmt.Println("Saved notification channels")
}

// Push Functions

func pushDashboards() {
	fmt.Println("Pushing dashboards...")
	dashboardDir := filepath.Join(directory, "dashboards")
	files, _ := ioutil.ReadDir(dashboardDir)

	var folderID string
	if folder != "" {
		folderID = getFolderID(folder)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			data, err := ioutil.ReadFile(filepath.Join(dashboardDir, file.Name()))
			if err != nil {
				fmt.Printf("Error reading dashboard file %s: %v\n", file.Name(), err)
				continue
			}

			var dashboard map[string]interface{}
			err = json.Unmarshal(data, &dashboard)
			if err != nil {
				fmt.Printf("Error unmarshalling dashboard %s: %v\n", file.Name(), err)
				continue
			}

			// Associa il dashboard alla cartella, se specificata
			if folderID != "" {
				if dashboard["folderId"] == nil || dashboard["folderId"] == 0 {
					dashboard["folderId"] = folderID
				}
			}

			// Imposta il payload per Grafana
			payload := map[string]interface{}{
				"dashboard": dashboard,
				"overwrite": true, // Sovrascrive il dashboard esistente
			}
			payloadData, _ := json.Marshal(payload)

			url := fmt.Sprintf("%s/api/dashboards/db", baseURL)
			sendRequest("POST", url, payloadData)
			fmt.Printf("Uploaded dashboard: %s\n", file.Name())
		}
	}
}

func pushDatasources() {
	fmt.Println("Pushing datasources...")
	datasourceFile := filepath.Join(directory, "datasources", "datasources.json")
	data, err := ioutil.ReadFile(datasourceFile)
	if err != nil {
		fmt.Println("Error reading datasources file:", err)
		return
	}

	var datasources []map[string]interface{}
	err = json.Unmarshal(data, &datasources)
	if err != nil {
		fmt.Println("Error unmarshalling datasources:", err)
		return
	}

	for _, ds := range datasources {
		dsJSON, _ := json.Marshal(ds)
		url := fmt.Sprintf("%s/api/datasources", baseURL)
		sendRequest("POST", url, dsJSON)
		fmt.Printf("Uploaded datasource: %s\n", ds["name"])
	}
}

func pushFolders() {
	fmt.Println("Pushing folders...")
	folderFile := filepath.Join(directory, "folders", "folders.json")
	data, err := ioutil.ReadFile(folderFile)
	if err != nil {
		fmt.Println("Error reading folders file:", err)
		return
	}

	var folders []map[string]interface{}
	err = json.Unmarshal(data, &folders)
	if err != nil {
		fmt.Println("Error unmarshalling folders:", err)
		return
	}

	for _, folder := range folders {
		folderJSON, _ := json.Marshal(folder)
		url := fmt.Sprintf("%s/api/folders", baseURL)
		sendRequest("POST", url, folderJSON)
		fmt.Printf("Uploaded folder: %s\n", folder["title"])
	}
}

func pushNotificationChannels() {
	fmt.Println("Pushing notification channels...")
	notificationFile := filepath.Join(directory, "notifications", "notifications.json")
	data, err := ioutil.ReadFile(notificationFile)
	if err != nil {
		fmt.Println("Error reading notifications file:", err)
		return
	}

	var notifications []map[string]interface{}
	err = json.Unmarshal(data, &notifications)
	if err != nil {
		fmt.Println("Error unmarshalling notifications:", err)
		return
	}

	for _, nc := range notifications {
		ncJSON, _ := json.Marshal(nc)
		url := fmt.Sprintf("%s/api/alert-notifications", baseURL)
		sendRequest("POST", url, ncJSON)
		fmt.Printf("Uploaded notification channel: %s\n", nc["name"])
	}
}

// Helper Functions

func downloadDashboard(uid string) []byte {
	url := fmt.Sprintf("%s/api/dashboards/uid/%s", baseURL, uid)
	return sendRequest("GET", url, nil)
}

func sendRequest(method, url string, body []byte) []byte {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		fmt.Println("Error creating request:", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("Error: %s returned %d\n", url, resp.StatusCode)
		os.Exit(1)
	}

	data, _ := ioutil.ReadAll(resp.Body)
	return data
}

func saveToFile(filePath string, data []byte) error {
	return ioutil.WriteFile(filePath, data, 0644)
}

