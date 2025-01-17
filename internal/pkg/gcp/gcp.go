// NOTICE:
// This code was created under resource constraints for GCP Functions SDK interaction.
// If reusing/publishing any part, please attribute it to me (@chaosRez) and cite our paper (see project README.md).
package GCP

import (
	"archive/zip"
	"cloud.google.com/go/iam/apiv1/iampb"
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/functions/apiv2"
	"cloud.google.com/go/functions/apiv2/functionspb"
	"cloud.google.com/go/run/apiv2"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type GCP struct {
	functionsClient *functions.FunctionClient
	projectID       string
	Location        string // NOTE Location is for Function or a service, not GCP client
}
type Function struct {
	Name                 string
	Location             string
	SourceZipURL         string // first priority is SourceZipURL then local and then git repo
	SourceLocalPath      string
	SourceGitRepoURL     string
	EntryPoint           string
	Runtime              string
	EnvironmentVariables map[string]string
}

func NewGCP(ctx context.Context, projectID, funcLocation string, credsPath string) (*GCP, error) {
	log.Infof("Initializing GCP client for project: %s", projectID)
	err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credsPath)
	if err != nil {
		return nil, err
	}
	log.Infof("Using credentials from: %s", credsPath)
	client, err := functions.NewFunctionClient(ctx, option.WithEndpoint("cloudfunctions.googleapis.com:443"))

	if err != nil {
		return nil, err
	}
	return &GCP{functionsClient: client, projectID: projectID, Location: funcLocation}, nil
}

// CreateFunction creates a new function with a given function either from a remote zip URL, a local path (Dir or Zip), or a Git repo URL
func (g *GCP) CreateFunction(ctx context.Context, f *Function) (string, error) { // Note: gives an error if the function already exists
	log.Infof("Creating %s function in %s", f.Name, f.Location)
	parent := fmt.Sprintf("projects/%s/locations/%s", g.projectID, f.Location)

	source, err := g.prepareSource(ctx, f, parent)
	if err != nil {
		return "", err
	}

	req := &functionspb.CreateFunctionRequest{
		Parent: parent,
		Function: &functionspb.Function{
			Name: fmt.Sprintf("%s/functions/%s", parent, f.Name),
			BuildConfig: &functionspb.BuildConfig{
				Source:               source,
				EntryPoint:           f.EntryPoint,
				Runtime:              f.Runtime,
				EnvironmentVariables: f.EnvironmentVariables, // build env variable
			},
			ServiceConfig: &functionspb.ServiceConfig{
				IngressSettings:      functionspb.ServiceConfig_ALLOW_ALL, // still needs authentication to access
				EnvironmentVariables: f.EnvironmentVariables,              // runtime env variable
			},
		},
		FunctionId: f.Name,
	}

	log.Infof("Sending CreateFunction request for function: %s", f.Name)
	op, err := g.functionsClient.CreateFunction(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed calling CreateFunction: %v", err)
	}

	_, err = op.Wait(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to wait for function creation: %v", err)
	}

	// Set IAM policy to allow unauthenticated access
	serviceName := fmt.Sprintf("projects/%s/locations/%s/services/%s", g.projectID, f.Location, f.Name)
	if err := setCloudRunIamPolicy(ctx, serviceName); err != nil {
		return "", fmt.Errorf("failed to set IAM policy: %v", err)
	}

	// Get the function details to retrieve the endpoint address
	getFunctionReq := &functionspb.GetFunctionRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/functions/%s", g.projectID, f.Location, f.Name),
	}
	function, err := g.functionsClient.GetFunction(ctx, getFunctionReq)
	if err != nil {
		return "", fmt.Errorf("failed to get function details: %v", err)
	}

	log.Infof("Function %s created successfully: %v", f.Name, function.ServiceConfig.Uri)
	return function.ServiceConfig.Uri, nil
}

func (g *GCP) UpdateFunction(ctx context.Context, f *Function) (string, error) {
	log.Infof("Updating %s function in %s", f.Name, f.Location)
	parent := fmt.Sprintf("projects/%s/locations/%s", g.projectID, f.Location)

	source, err := g.prepareSource(ctx, f, parent)
	if err != nil {
		return "", err
	}

	req := &functionspb.UpdateFunctionRequest{
		Function: &functionspb.Function{
			Name: fmt.Sprintf("%s/functions/%s", parent, f.Name),
			BuildConfig: &functionspb.BuildConfig{
				Source:               source,
				EntryPoint:           f.EntryPoint,
				Runtime:              f.Runtime,
				EnvironmentVariables: f.EnvironmentVariables, // build env variable
			},
			ServiceConfig: &functionspb.ServiceConfig{
				IngressSettings:      functionspb.ServiceConfig_ALLOW_ALL,
				EnvironmentVariables: f.EnvironmentVariables, // runtime env variable
			},
		},
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{ // Update only the specified fields if already exists
				"build_config.source",
				"build_config.entry_point",
				"build_config.runtime",
				"build_config.environment_variables",
				"service_config.environment_variables",
			},
		},
	}

	log.Infof("Sending UpdateFunction request for function: %s", f.Name)
	op, err := g.functionsClient.UpdateFunction(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed calling UpdateFunction: %v", err)
	}

	_, err = op.Wait(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to wait for function update: %v", err)
	}

	// FIXME: is this necessary?
	serviceName := fmt.Sprintf("projects/%s/locations/%s/services/%s", g.projectID, f.Location, f.Name)
	if err := setCloudRunIamPolicy(ctx, serviceName); err != nil {
		return "", fmt.Errorf("failed to set IAM policy: %v", err)
	}

	getFunctionReq := &functionspb.GetFunctionRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/functions/%s", g.projectID, f.Location, f.Name),
	}
	function, err := g.functionsClient.GetFunction(ctx, getFunctionReq)
	if err != nil {
		return "", fmt.Errorf("failed to get function details: %v", err)
	}

	log.Infof("Function %s updated successfully. Endpoint: %s", f.Name, function.ServiceConfig.Uri)
	return function.ServiceConfig.Uri, nil
}

func (g *GCP) DeleteFunction(ctx context.Context, f *Function) error {
	log.Infof("Deleting function: %s in location: %s", f.Name, f.Location)
	name := fmt.Sprintf("projects/%s/locations/%s/functions/%s", g.projectID, f.Location, f.Name)
	req := &functionspb.DeleteFunctionRequest{
		Name: name,
	}

	log.Infof("Sending DeleteFunction request for function: %s", f.Name)
	op, err := g.functionsClient.DeleteFunction(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete function: %v", err)
	}

	err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for function deletion: %v", err)
	}

	log.Infof("Function %s deleted successfully", f.Name)
	return nil
}

func (g *GCP) GetFunction(ctx context.Context, f *Function) (*functionspb.Function, error) {
	log.Infof("Getting details for function: %s in location: %s", f.Name, f.Location)
	req := &functionspb.GetFunctionRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/functions/%s", g.projectID, f.Location, f.Name),
	}

	function, err := g.functionsClient.GetFunction(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get function details: %v", err)
	}

	log.Debugf("Function '%s' details retrieved successfully: %v", f.Name, function)
	return function, nil
}

func (g *GCP) Close() error {
	log.Info("Closing GCP client...")
	err := g.functionsClient.Close()
	if err != nil {
		return fmt.Errorf("Failed to close GCP client: %v", err)
	}
	log.Info("GCP client closed successfully")
	return nil
}

// --- Private helper functions ---

func (g *GCP) prepareSource(ctx context.Context, f *Function, parent string) (*functionspb.Source, error) {
	var source *functionspb.Source
	if f.SourceZipURL != "" {
		log.Infof("Using function source from remote ZIP URL: %s", f.SourceZipURL)
		uploadURLResponse, err := g.functionsClient.GenerateUploadUrl(ctx, &functionspb.GenerateUploadUrlRequest{
			Parent: parent,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate upload URL: %v", err)
		}
		log.Debugf("Generated upload URL: %s", uploadURLResponse.GetUploadUrl())

		// Download the ZIP file directly from the remote URL
		resp, err := http.Get(f.SourceZipURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download ZIP source: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download ZIP source, status code: %d", resp.StatusCode)
		}

		err = uploadZipToTempBucket(ctx, resp.Body, uploadURLResponse.GetUploadUrl())
		if err != nil {
			return nil, fmt.Errorf("failed to download and upload ZIP: %v", err)
		}

		uploadURL := uploadURLResponse.GetUploadUrl()
		u, err := url.Parse(uploadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse upload URL: %v", err)
		}

		parts := strings.Split(u.Path[1:], "/") // Remove leading slash then split by '/'
		bucket := parts[0]
		object := parts[1]
		log.Infof("Uploaded file to bucket: %s, object: %s", bucket, object)

		source = &functionspb.Source{
			Source: &functionspb.Source_StorageSource{
				StorageSource: &functionspb.StorageSource{
					Bucket: bucket,
					Object: object,
				},
			},
		}
	} else if f.SourceLocalPath != "" {
		log.Infof("Using function source from local file path: %s", f.SourceLocalPath)
		uploadURLResponse, err := g.functionsClient.GenerateUploadUrl(ctx, &functionspb.GenerateUploadUrlRequest{
			Parent: parent,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate upload URL: %v", err)
		}
		log.Debugf("Generated upload URL: %s", uploadURLResponse.GetUploadUrl())

		// Upload the local zip/folder to the generated uploadURL
		var file *os.File
		fileInfo, err := os.Stat(f.SourceLocalPath)
		if err != nil {
			pwd, _ := os.Getwd()
			return nil, fmt.Errorf("failed to stat local path from %v: %v", pwd, err)
		}
		if fileInfo.IsDir() {
			log.Infof("Temporary zipping directory: %s", f.SourceLocalPath)
			file, err = zipDirectory(f.SourceLocalPath)
			log.Debugf("zip file: %v", file.Name())
			if err != nil {
				return nil, fmt.Errorf("failed to zip directory: %v", err)
			}
			defer os.Remove(file.Name()) // Clean up the temp zip file
		} else {
			file, err = os.Open(f.SourceLocalPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open local file: %v", err)
			}
		}
		defer file.Close()

		err = uploadZipToTempBucket(ctx, file, uploadURLResponse.GetUploadUrl())
		if err != nil {
			return nil, fmt.Errorf("failed to upload local ZIP: %v", err)
		}

		// Extract bucket and object from upload URL
		uploadURL := uploadURLResponse.GetUploadUrl()
		u, err := url.Parse(uploadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse upload URL: %v", err)
		}
		parts := strings.Split(u.Path[1:], "/") // Remove leading slash then split by '/'
		bucket := parts[0]
		object := parts[1]
		log.Infof("Uploaded file to bucket: %s, object: %s", bucket, object)

		source = &functionspb.Source{
			Source: &functionspb.Source_StorageSource{
				StorageSource: &functionspb.StorageSource{
					Bucket: bucket,
					Object: object,
				},
			},
		}
	} else if f.SourceGitRepoURL != "" {
		log.Infof("Using function source from Git repo URL: %s", f.SourceGitRepoURL)
		u, err := url.Parse(f.SourceGitRepoURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Git repo URL: %v", err)
		}

		// Assuming GitHub URL format: https://github.com/{owner}/{repo_name}/tree/{branch_name}/{dir}
		pathParts := strings.Split(u.Path, "/")
		if len(pathParts) < 5 || pathParts[3] != "tree" {
			return nil, fmt.Errorf("invalid GitHub URL format")
		}

		repoName := pathParts[2]
		branchName := pathParts[4]
		dir := strings.Join(pathParts[5:], "/")

		source = &functionspb.Source{
			Source: &functionspb.Source_RepoSource{
				RepoSource: &functionspb.RepoSource{
					ProjectId: g.projectID,
					RepoName:  repoName,
					Dir:       dir,
					Revision: &functionspb.RepoSource_BranchName{
						BranchName: branchName,
					},
				},
			},
		}
	} else {
		return nil, fmt.Errorf("no source code location specified")
	}
	return source, nil
}

func uploadZipToTempBucket(ctx context.Context, zipReader io.Reader, uploadURL string) error {
	log.Debugf("Uploading ZIP file to: %s", uploadURL)
	req, err := http.NewRequest("PUT", uploadURL, zipReader)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %v", err)
	}
	req.Header.Set("Content-Type", "application/zip") // necessary

	client := &http.Client{}
	uploadResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload ZIP file: %v", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload ZIP file, status code: %d", uploadResp.StatusCode)
	}

	return nil
}

// Helper function to create a ZIP archive from a directory
func zipDirectory(source string) (*os.File, error) {
	zipFile, err := os.CreateTemp("", "source-*.zip")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			_, err = zipWriter.Create(relPath + "/")
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(writer, file)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to zip directory: %v", err)
	}

	// Ensure the zip file is written to disk
	err = zipWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %v", err)
	}

	// Reopen the file to ensure it's written correctly
	zipFile, err = os.Open(zipFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to reopen zip file: %v", err)
	}

	// read back the zip file to make sure if it is not corrupted
	zipContents, err := io.ReadAll(zipFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read back zip file: %v", err)
	}
	log.Debugf("success to read back the ZIP file. First 50 bytes contents: %x", zipContents[:50]) // Print first 100 bytes for inspection

	// Reset the file pointer to the beginning
	_, err = zipFile.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to reset zip file pointer: %v", err)
	}

	return zipFile, nil
}

// 2nd gen functions are Cloud Run services, so we need to set the IAM policy for the service to be publicly available
func setCloudRunIamPolicy(ctx context.Context, serviceName string) error {
	log.Infof("Setting IAM policy to allUsers for Cloud Run service: %s", serviceName)
	// Create a Cloud Run client
	runClient, err := run.NewServicesClient(ctx, option.WithEndpoint("run.googleapis.com:443"))
	if err != nil {
		return fmt.Errorf("failed to create Cloud Run client: %v", err)
	}
	defer runClient.Close()

	// Get the existing IAM policy
	getPolicyReq := &iampb.GetIamPolicyRequest{
		Resource: serviceName,
	}
	policy, err := runClient.GetIamPolicy(ctx, getPolicyReq)
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %v", err)
	}

	// Add a binding for the allUsers principal
	policy.Bindings = append(policy.Bindings, &iampb.Binding{
		Role:    "roles/run.invoker",
		Members: []string{"allUsers"},
	})

	// Set the updated IAM policy
	setPolicyReq := &iampb.SetIamPolicyRequest{
		Resource: serviceName,
		Policy:   policy,
	}
	_, err = runClient.SetIamPolicy(ctx, setPolicyReq)
	if err != nil {
		return fmt.Errorf("failed to set IAM policy: %v", err)
	}

	return nil
}

func main() {
	log.SetLevel(log.DebugLevel)
	ctx := context.Background()
	projectID := "geofaas-411316"

	gcp, err := NewGCP(ctx, projectID, "europe-west10", "config/gcp-user-creds.json")
	if err != nil {
		log.Fatalf("Failed to initialize GCP client: %v", err)
	}
	defer gcp.Close()

	function := &Function{
		Name:     "sieve",
		Location: "europe-west10",
		//SourceLocalPath: "../faas/tinyfaas/test/fns/sieve-of-eratosthenes",
		SourceLocalPath: "../umbilical-choir-proxy/binary/_gcp-amd64",
		//SourceLocalPath: "../faas/tinyfaas/test/fns/sieve.zip",
		//SourceZipURL:         "https://github.com/OpenFogStack/tinyFaas/archive/main.zip",
		//SourceGitRepoURL
		//EntryPoint: "http", // if is nodejs it will look for a index.js and will infer the entry point if module.export is used
		//Runtime:    "nodejs20",
		Runtime:    "python312",
		EntryPoint: "fn",
		EnvironmentVariables: map[string]string{
			"PORT":    "8000",
			"HOST":    "172.17.0.1",
			"F1NAME":  fmt.Sprintf("%s", ""),
			"F2NAME":  fmt.Sprintf("%s", ""),
			"PROGRAM": fmt.Sprintf("ab-%s", ""),
			"BCHANCE": fmt.Sprintf("%v", ""),
		},
	}

	if _, err := gcp.CreateFunction(ctx, function); err != nil {
		log.Fatalf("Failed to create function: %v", err)
	}

	//if uri, err := gcp.Update(ctx, function); err != nil {
	//	log.Fatalf("Failed to update function: %v", err)
	//}
	//
	//if uri, err := gcp.DeleteFunction(ctx, function); err != nil {
	//	log.Fatalf("Failed to delete function: %v", err)
	//}
}
