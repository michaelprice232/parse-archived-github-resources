package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

const targetTFModuleName = "terraform-module-github-repository"

func main() {
	root := flag.String("root-dir", "../terraform-github/terraform/repos", "Root directory to start processing Terraform files from (one directory level down)")
	outputDir := flag.String("output-dir", "./output", "Directory the Terragrunt state removal scripts should be written to")
	flag.Parse()
	rootDirectory := *root

	tfFiles, err := getTerraformFilesInDirectories(rootDirectory)
	if err != nil {
		log.Fatalf("error whilst listing Terraform files from root directory %s: %v", rootDirectory, err)
	}

	err = generateArchivedReposRemoval(rootDirectory, *outputDir, tfFiles)
	if err != nil {
		log.Fatalf("error whilst generating archived repositories state removal output: %v", err)
	}
}

type terraformFile struct {
	fileName      string
	directoryName string
}

// generateArchivedReposRemoval processing each Terraform file by calling processTerraformFile and then writing a bash script to outputDir
// which includes the Terragrunt state removal commands.
func generateArchivedReposRemoval(rootDir, outputDir string, tfFiles []terraformFile) error {
	for _, tfFile := range tfFiles {
		targetFile := filepath.Join(rootDir, tfFile.directoryName, tfFile.fileName)
		fmt.Printf("Processing file: %s\n", targetFile)

		archivedResources, err := processTerraformFile(targetFile)
		if err != nil {
			return fmt.Errorf("error whilst processing file %s: %v", targetFile, err)
		}

		// Skip any TF files which do not contain any archived GitHub resources
		if len(archivedResources) == 0 {
			fmt.Printf("No archived repositories found for file %s\n", targetFile)
			continue
		}

		// Write results to file
		outputFileName := fmt.Sprintf("%s-%s.sh", tfFile.directoryName, strings.TrimSuffix(tfFile.fileName, ".tf"))
		outputFilePath := filepath.Join(outputDir, outputFileName)

		f, err := os.Create(outputFilePath)
		if err != nil {
			return fmt.Errorf("error whilst creating file %s: %v", outputFilePath, err)
		}

		for _, archivedResource := range archivedResources {
			_, err = f.WriteString(fmt.Sprintf("terragrunt state rm %s\n", archivedResource))
			if err != nil {
				return fmt.Errorf("error whilst writing to file %s: %v", outputFilePath, err)
			}
		}
		err = f.Close()
		if err != nil {
			return fmt.Errorf("error whilst closing file %s: %v", outputFilePath, err)
		}
	}

	return nil
}

// getTerraformFilesInDirectories returns all the Terraform (*.tf) files one directory down from the rootPath.
// Filters for only Terraform files which include github_repository references, either explicit or in a module.
// Excludes config.tf files as these are symlinked and contain target strings in comments which is causing false positives.
func getTerraformFilesInDirectories(rootPath string) ([]terraformFile, error) {
	results := make([]terraformFile, 0)

	rootEntries, err := os.ReadDir(rootPath)
	if err != nil {
		return results, fmt.Errorf("failed to read root directory: %w", err)
	}

	for _, entry := range rootEntries {
		if entry.IsDir() {
			childDirectoryPath := fmt.Sprintf("%s/%s", rootPath, entry.Name())

			childDirEntries, err := os.ReadDir(childDirectoryPath)
			if err != nil {
				return results, fmt.Errorf("failed to read child directory %s: %w", childDirectoryPath, err)
			}

			for _, childDirEntry := range childDirEntries {
				// Only process *.tf files
				if strings.HasSuffix(childDirEntry.Name(), ".tf") {

					// Check that the file contains either a module reference or a github_repository reference.
					// Also exclude config.tf as this is a symlink and causes a false positive due to a string being the comments.
					body, err := os.ReadFile(filepath.Join(childDirectoryPath, childDirEntry.Name()))
					if err != nil {
						return results, fmt.Errorf("failed to read child directory %s: %w", childDirectoryPath, err)
					}
					if (!strings.Contains(string(body), "github_repository") && !strings.Contains(string(body), targetTFModuleName)) || childDirEntry.Name() == "config.tf" {
						continue
					}

					results = append(results, terraformFile{
						fileName:      childDirEntry.Name(),
						directoryName: entry.Name(),
					})
				}
			}
		}
	}

	return results, nil
}

// processTerraformFile reads a Terraform file and returns the github_repository resources which are archived.
// This includes ones which are embedded in Terraform modules named targetTFModuleName.
func processTerraformFile(configFile string) ([]string, error) {
	results := make([]string, 0)

	var ok bool
	var attr *hclsyntax.Attribute

	parser := hclparse.NewParser()

	// Parse the HCL file
	file, diags := parser.ParseHCLFile(configFile)
	if (diags != nil && diags.HasErrors()) || file == nil {
		return results, fmt.Errorf("failed to parse HCL file '%s': %s", configFile, diags)
	}

	// Parse the file body as an HCL syntax body
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return results, fmt.Errorf("%s: failed to cast file body to HCL syntax body", configFile)
	}

	for _, block := range body.Blocks {

		// Standalone github_repository resources
		if block.Type == "resource" && block.Labels[0] == "github_repository" {
			resourceName := block.Labels[1]

			// Check whether the archived attribute is set to true. Skip if there is no archived attribute
			if attr, ok = block.Body.Attributes["archived"]; !ok {
				continue
			}

			val, diags := attr.Expr.Value(&hcl.EvalContext{})
			if diags != nil && diags.HasErrors() {
				return results, fmt.Errorf("%s: failed to evaluate attribute expression: %s", configFile, diags)
			}

			// Convert to a boolean
			archived := val.True()

			if archived {
				results = append(results, fmt.Sprintf("%s.%s", block.Labels[0], resourceName))
			}
		}

		//  Module containing github_repository resources
		if block.Type == "module" {
			correctModule := false
			resourceName := block.Labels[0]

			if attr, ok = block.Body.Attributes["source"]; !ok {
				continue
			}
			val, diags := attr.Expr.Value(&hcl.EvalContext{})
			if diags != nil && diags.HasErrors() {
				return results, fmt.Errorf("%s: Failed to evaluate attribute expression: %s", configFile, diags)
			}

			// Convert to string
			moduleSource := val.AsString()

			// Check for specific Terraform module name
			if strings.Contains(moduleSource, targetTFModuleName) {
				correctModule = true
			}

			// Skip if there is no archived attribute
			if attr, ok = block.Body.Attributes["archived"]; !ok {
				continue
			}

			// Skip if the module source doesn't reference the target one
			if correctModule {
				val, diags := attr.Expr.Value(&hcl.EvalContext{})
				if diags != nil && diags.HasErrors() {
					return results, fmt.Errorf("%s: failed to evaluate attribute expression: %s", configFile, diags)
				}

				// Convert to a boolean
				archived := val.True()

				if archived {
					results = append(results, fmt.Sprintf("module.%s", resourceName))
				}
			}
		}
	}

	return results, nil
}
