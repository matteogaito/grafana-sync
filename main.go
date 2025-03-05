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
	"context"
	"log"

	"github.com/grafana-tools/sdk"
)

var (
	apiKey    string
	baseURL   string
	directory string
	action    string
	folder    string
	client    *sdk.Client
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

	client, _ = sdk.NewClient(baseURL, apiKey, sdk.DefaultHTTPClient)
	if client == nil {
		log.Fatalf("Error: failed to initialize Grafana client")
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

// Helper to get folder ID by name
func getFolderID(folderName string) int {
	ctx := context.Background()
	folders, err := client.GetAllFolders(ctx)
	if err != nil {
		log.Fatalf("Error fetching folders: %v", err)
	}

	for _, f := range folders {
		if f.Title == folderName {
			return f.ID
		}
	}

	log.Fatalf("Folder not found: %s", folderName)
	return 0
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
	ctx := context.Background()

	searchParams := []sdk.SearchParam{sdk.SearchType(sdk.SearchTypeDashboard)}
	if folder != "" {
		folderID := getFolderID(folder)
		searchParams = append(searchParams, sdk.SearchFolderID(int(folderID)))
	}

	// Search for dashboards using the client
	dashboards, err := client.Search(ctx, searchParams...)
	if err != nil {
		log.Fatalf("Error searching dashboards: %v", err)
	}

	// Create local directory for dashboards
	dashboardDir := filepath.Join(directory, "dashboards")
	if err := os.MkdirAll(dashboardDir, os.ModePerm); err != nil {
		log.Fatalf("Error creating directory: %v", err)
	}

	// Iterate through dashboards and save them locally
	for _, db := range dashboards {
		if db.Type != "dash-db" {
			continue // Skip non-dashboard entries
		}

		// Fetch the full dashboard using UID
		board, meta, err := client.GetDashboardByUID(ctx, db.UID)
		if err != nil {
			log.Printf("Error fetching dashboard UID %s: %v", db.UID, err)
			continue
		}

		// Ensure the dashboard has a title
		if board.Title == "" {
			log.Printf("Error: dashboard UID %s has no title", db.UID)
			continue
		}

		// removing uniq identifier
		board.ID = 0
		fmt.Println(board.ID)

		// Save the dashboard as a JSON file
		filePath := filepath.Join(dashboardDir, meta.Slug+".json")
		data, err := json.MarshalIndent(board, "", "  ")
		if err != nil {
			log.Printf("Error marshaling dashboard UID %s: %v", db.UID, err)
			continue
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			log.Printf("Error saving dashboard UID %s: %v", db.UID, err)
			continue
		}

		fmt.Printf("Saved dashboard: %s\n", filePath)
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
	ctx := context.Background()

	// Read the local dashboards directory
	dashboardDir := filepath.Join(directory, "dashboards")
	files, err := os.ReadDir(dashboardDir)
	if err != nil {
		log.Fatalf("Error reading dashboard directory: %v", err)
	}

	// Get folder ID if a folder is specified
	var folderID int
	if folder != "" {
		folderID = getFolderID(folder)
		fmt.Printf("Using folder ID: %d for dashboards\n", folderID)
	}

	// Iterate through dashboard files
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			filePath := filepath.Join(dashboardDir, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Error reading file %s: %v", file.Name(), err)
				continue
			}

			// Unmarshal the JSON into a Board struct
			var dashboard sdk.Board
			if err := json.Unmarshal(data, &dashboard); err != nil {
				log.Printf("Error unmarshalling file %s: %v", file.Name(), err)
				continue
			}

			params := sdk.SetDashboardParams{
				FolderID:  folderID,
				Overwrite: true,      // Enable overwriting existing dashboards
			}

			// Push the dashboard to Grafana
			fmt.Printf("Pushing dashboard %s - %s in %d\n", dashboard.Title, dashboard.UID, folderID)
			_, err = client.SetDashboard(ctx, dashboard, params)
			if err != nil {
				log.Printf("Error pushing dashboard %s: %v", file.Name(), err)
				continue
			}

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

