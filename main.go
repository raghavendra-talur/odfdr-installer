package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

//go:embed icsp.yaml
var icspYAML string

//go:embed odf-catalogsource.yaml
var odfCatalogSourceYAML string

// checkCommandExists verifies that a required command is available in the system path
func checkCommandExists(command string) error {
	_, err := exec.LookPath(command)
	if err != nil {
		return fmt.Errorf("%s is not installed or not in PATH", command)
	}
	return nil
}

// checkRequiredCommands verifies that all required commands are available
func checkRequiredCommands() error {
	requiredCommands := []string{"jq", "oc"}

	for _, cmd := range requiredCommands {
		if err := checkCommandExists(cmd); err != nil {
			return err
		}
	}

	return nil
}

func getClusterName(url string) (string, error) {
	parts := strings.Split(url, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("could not parse cluster name from URL")
	}

	return parts[1], nil
}

func getKubeconfig(cluster string) (*os.File, error) {
	kconfig, err := os.CreateTemp("", cluster+"-kubeconfig"+"-*")
	if err != nil {
		return nil, err
	}
	defer kconfig.Close()

	return kconfig, nil
}

func login(url, username, password, kconfig string) error {
	loginCmd := exec.Command("oc", "login", url, "-u", username, "-p", password)
	loginCmd.Stdout = os.Stdout
	loginCmd.Stderr = os.Stderr
	loginCmd.Env = append(os.Environ(), "KUBECONFIG="+kconfig)

	slog.Info("logging in using kubeconfig", "kubeconfig", kconfig, "cluster", url)

	err := loginCmd.Run()
	if err != nil {
		return fmt.Errorf("error logging into OpenShift: %v", err)
	}

	return nil
}

func showUsage() {
	fmt.Println("Usage: ./odfdr-installer -url <URL> -username <username> -password <password> -rhceph-password <password>")
	fmt.Println("Example: ./odfdr-installer -url ./odfdr-installer -url api.cluster.example.com:6443 -password abc -rhceph-password=xyz")
}

func showUsageAndExit() {
	showUsage()
	os.Exit(1)
}

func addCatalogSource(clusterName, kconfig, catalogSourceYAML string) error {
	catalogSourceFileName := clusterName + "-catalogsource.yaml"
	err := os.WriteFile(catalogSourceFileName, []byte(catalogSourceYAML), 0o644)
	if err != nil {
		return fmt.Errorf("error writing CatalogSource to file: %v", err)
	}

	applyCmd := exec.Command("oc", "apply", "-f", catalogSourceFileName)
	applyCmd.Env = append(os.Environ(), "KUBECONFIG="+kconfig)
	err = applyCmd.Run()
	if err != nil {
		return fmt.Errorf("error applying CatalogSource: %v", err)
	}

	return nil
}

func addICSP(clusterName, kconfig string) error {
	icspFileName := clusterName + "-icsp.yaml"
	err := os.WriteFile(icspFileName, []byte(icspYAML), 0o644)
	if err != nil {
		return fmt.Errorf("error writing ICSP to file: %v", err)
	}

	applyCmd := exec.Command("oc", "apply", "-f", icspFileName)
	applyCmd.Env = append(os.Environ(), "KUBECONFIG="+kconfig)
	err = applyCmd.Run()
	if err != nil {
		return fmt.Errorf("error applying ICSP: %v", err)
	}

	return nil
}

func addRHCEPHAuth(clusterName, kconfig, rhcephPassword string) error {
	getPullSecretCmd := exec.Command("oc", "get", "secret/pull-secret", "-n", "openshift-config", "--template={{index .data \".dockerconfigjson\" | base64decode}}")
	getPullSecretCmd.Env = append(os.Environ(), "KUBECONFIG="+kconfig)
	pullSecretOutput, err := getPullSecretCmd.Output()
	if err != nil {
		return fmt.Errorf("error getting pull secret: %v", err)
	}

	pullSecretFileName := clusterName + "-pull-secret.json"
	err = os.WriteFile(pullSecretFileName, pullSecretOutput, 0o644)
	if err != nil {
		return fmt.Errorf("error writing pull secret to file: %v", err)
	}

	var pullSecret map[string]any
	err = json.Unmarshal(pullSecretOutput, &pullSecret)
	if err != nil {
		return fmt.Errorf("error parsing pull secret JSON: %v", err)
	}

	auths, ok := pullSecret["auths"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid pull secret format")
	}
	elementsCount := len(auths)

	if pullSecret["auths"] == nil {
		return fmt.Errorf("pull secret does not contain auths")
	}

	if pullSecret["auths"].(map[string]any)["quay.io/rhceph-dev"] != nil {
		slog.Info("RHCEPH auth already exists in pull secret")
		return nil
	}

	appendFileName := clusterName + "-append-pull-secret.json"
	registryLoginCmd := exec.Command("oc", "registry", "login", "--registry=quay.io/rhceph-dev",
		"--auth-basic="+rhcephPassword, "--to="+appendFileName)
	registryLoginCmd.Env = append(os.Environ(), "KUBECONFIG="+kconfig)
	err = registryLoginCmd.Run()
	if err != nil {
		return fmt.Errorf("error logging into registry: %v", err)
	}

	newPullSecretFileName := clusterName + "-new-pull-secret.json"
	mergeCmd := exec.Command("jq", "-s", ".[0] * .[1]", pullSecretFileName, appendFileName)
	mergedOutput, err := mergeCmd.Output()
	if err != nil {
		return fmt.Errorf("error merging pull secrets: %v", err)
	}

	err = os.WriteFile(newPullSecretFileName, mergedOutput, 0o644)
	if err != nil {
		return fmt.Errorf("error writing merged pull secret to file: %v", err)
	}

	updateCmd := exec.Command("oc", "set", "data", "secret/pull-secret", "-n", "openshift-config",
		"--from-file=.dockerconfigjson="+newPullSecretFileName)
	updateCmd.Env = append(os.Environ(), "KUBECONFIG="+kconfig)
	err = updateCmd.Run()
	if err != nil {
		return fmt.Errorf("error updating pull secret: %v", err)
	}

	getPullSecretCmd = exec.Command("oc", "get", "secret/pull-secret", "-n", "openshift-config", "--template={{index .data \".dockerconfigjson\" | base64decode}}")
	getPullSecretCmd.Env = append(os.Environ(), "KUBECONFIG="+kconfig)
	pullSecretOutput, err = getPullSecretCmd.Output()
	if err != nil {
		return fmt.Errorf("error getting pull secret: %v", err)
	}

	var newPullSecret map[string]any
	err = json.Unmarshal(mergedOutput, &newPullSecret)
	if err != nil {
		return fmt.Errorf("error parsing new pull secret JSON: %v", err)
	}

	newAuths, ok := newPullSecret["auths"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid new pull secret format")
	}

	newElementsCount := len(newAuths)

	if newElementsCount != elementsCount+1 {
		return fmt.Errorf("pull secret does not contain the expected number of elements")
	}

	return nil
}

func main() {
	urlFlag := flag.String("url", "", "OpenShift API URL")
	usernameFlag := flag.String("username", "kubeadmin", "OpenShift username")
	passwordFlag := flag.String("password", "", "OpenShift password")
	rhcephPasswordFlag := flag.String("rhceph-password", "", "RHCEPH repository password")

	flag.Parse()

	if *urlFlag == "" {
		slog.Error("error: URL is required")
		showUsageAndExit()
	}

	if *passwordFlag == "" {
		slog.Error("error: password is required")
		showUsageAndExit()
	}

	if *rhcephPasswordFlag == "" {
		slog.Error("error: RHCEPH password is required")
		showUsageAndExit()
	}

	url := *urlFlag
	username := *usernameFlag
	password := *passwordFlag
	rhcephPassword := *rhcephPasswordFlag

	if err := checkRequiredCommands(); err != nil {
		slog.Error("error checking required commands", "error", err)
		os.Exit(1)
	}

	clusterName, err := getClusterName(url)
	if err != nil {
		slog.Error("error getting cluster name", "error", err)
		os.Exit(1)
	}

	kconfig, err := getKubeconfig(clusterName)
	if err != nil {
		slog.Error("error creating kubeconfig file", "error", err)
		os.Exit(1)
	}

	if err := login(url, username, password, kconfig.Name()); err != nil {
		slog.Error("error logging into OpenShift", "error", err)
		os.Exit(1)
	}

	if err := addRHCEPHAuth(clusterName, kconfig.Name(), rhcephPassword); err != nil {
		slog.Error("error adding RHCEPH auth to pull secret", "error", err)
		os.Exit(1)
	}

	if err := addICSP(clusterName, kconfig.Name()); err != nil {
		slog.Error("error adding ICSP", "error", err)
		os.Exit(1)
	}

	if err := addCatalogSource(clusterName, kconfig.Name(), odfCatalogSourceYAML); err != nil {
		slog.Error("error adding CatalogSource", "error", err)
		os.Exit(1)
	}
}
