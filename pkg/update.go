package magellan

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OpenCHAMI/magellan/pkg/bmc"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

type UpdateParams struct {
	CollectParams
	URI              string   // Set from the positional paramters to update
	FirmwareURI      string   // set from the --firmware-url flag
	TransferProtocol string   // set from the --scheme flag
	Insecure         bool     // set from the --insecure flag
}

func UpdateFirmwareRemoteInsecure(q *UpdateParams) error {
	curlcmd := "curl"

	fmt.Println("INSECURE MODE (OpenBMC mode https://github.com/openbmc)")
	// first to copy in local the file that we want to uplaod to the BMC
	currentDir, err := os.Getwd()

	fmt.Printf("currentDir %s\n", currentDir)
	if err != nil {
		fmt.Println(err)
	}
	destPath := filepath.Join(currentDir, "newfw.mtd.tar")

	// fmt.Printf("destPath %s\n", destPath)

	// $ curl http://172.31.19.129:1337/obmc-phosphor-image-sipearl-evb0.static.mtd.tar --output foo
	//   % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
	//                                  Dload  Upload   Total   Spent    Left  Speed
	// 100 36.0M  100 36.0M    0     0   901M      0 --:--:-- --:--:-- --:--:--  924M

	cmd := exec.Command(curlcmd, "--output", destPath, q.FirmwareURI)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Printf("cmd.Run() failed with %s\n", err)
	} else {
		fmt.Println("Firmware update initiated successfully.")
	}

	// On OpenBMC we need to execute this kind of command
	// 	curl  -k -H "X-Auth-Token: $token"  -H "Content-Type:multipart/form-data" -X POST \
	//  -F UpdateParameters='{"Targets":["/redfish/v1/Managers/bmc"],"@Redfish.OperationApplyTime":"Immediate"};type=application/json' \
	//  -F 'UpdateFile=@obmc-phosphor-image-sipearl-evb0-20250304115722.static.mtd.tar;type=application/octet-stream' \
	// https://root:0penBmc@192.168.75.39/redfish/v1/UpdateService/update

	// Get BMC credentials from secret store in update parameters
	bmcCreds, err := bmc.GetBMCCredentials(q.SecretStore, q.URI)
	if err != nil {
		return fmt.Errorf("failed to get BMC credentials: %w", err)
	}
	uri, err := url.ParseRequestURI(q.URI)
	if err != nil {
		return fmt.Errorf("failed to parse URI: %w", err)
	}

	prefix := "//" + bmcCreds.Username + ":" + bmcCreds.Password+ "@"
	theurl := strings.Replace(uri.String(), "//", prefix, 1)
	theurl += "/redfish/v1/UpdateService/update"
	cmd = exec.Command(curlcmd, "-k", "-H \"X-Auth-Token: $token\"",
		"-H \"Content-Type:multipart/form-data\"", "-X", "POST",
		"-F", "UpdateFile=@newfw.mtd.tar;type=application/octet-stream",
		"-F", "UpdateParameters={\"Targets\":[\"/redfish/v1/Managers/bmc\"],\"@Redfish.OperationApplyTime\":\"Immediate\"}",
		theurl)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Printf("cmd.Run() failed with %s\n", err)
	} else {
		fmt.Println("Firmware update initiated successfully.")
	}
	// remove the copy of the BMC image
	err = os.Remove("newfw.mtd.tar")
	if err != nil {
		fmt.Printf("os.Remove failed with %s\n", err)
	}
	return nil;
}

// UpdateFirmwareRemote() uses 'gofish' to update the firmware of a BMC node.
// The function expects the firmware URL, firmware version, and component flags to be
// set from the CLI to perform a firmware update.
// Example:
// ./magellan/magellan update https://192.168.87.176:443 --username root
// --password 0penBmc
// --firmware-uri http://172.31.19.129:1337/obmc-phosphor-image-sipearl-evb0.static.mtd.tar
// --scheme TFTP --insecure
func UpdateFirmwareRemote(q *UpdateParams) error {
	// parse URI to set up full address
	uri, err := url.ParseRequestURI(q.URI)
	if err != nil {
		return fmt.Errorf("failed to parse URI: %w", err)
	}

	// Get BMC credentials from secret store in update parameters
	bmcCreds, err := bmc.GetBMCCredentials(q.SecretStore, q.URI)
	if err != nil {
		return fmt.Errorf("failed to get BMC credentials: %w", err)
	}

	if (q.Insecure) {
		fmt.Printf("INSECURE\n")
		UpdateFirmwareRemoteInsecure(q);
		return nil;
	} else {
		fmt.Printf("SECURE MODE\n")
	}

	// SECURE MODE
	// Connect to the Redfish service using gofish
	client, err := gofish.Connect(gofish.ClientConfig{Endpoint: uri.String(), Username: bmcCreds.Username, Password: bmcCreds.Password, Insecure: q.Insecure})
	if err != nil {
		return fmt.Errorf("failed to connect to Redfish service: %w", err)
	}
	defer client.Logout()

	// Retrieve the UpdateService from the Redfish client
	updateService, err := client.Service.UpdateService()
	if err != nil {
		return fmt.Errorf("failed to get update service: %w", err)
	}

	// Build the update request payload
	req := redfish.SimpleUpdateParameters{
		ForceUpdate:      true,
		ImageURI:         q.FirmwareURI,
		TransferProtocol: redfish.TransferProtocolType(q.TransferProtocol),
	}

	// Execute the SimpleUpdate action
	err = updateService.SimpleUpdate(&req)
	if err != nil {
		return fmt.Errorf("firmware update failed: %w", err)
	}
	fmt.Println("Firmware update initiated successfully.")

	return nil
}

func GetUpdateStatus(q *UpdateParams) error {
	// parse URI to set up full address
	uri, err := url.ParseRequestURI(q.URI)
	if err != nil {
		return fmt.Errorf("failed to parse URI: %w", err)
	}

	// Get BMC credentials from secret store in update parameters
	bmcCreds, err := bmc.GetBMCCredentials(q.SecretStore, q.URI)
	if err != nil {
		return fmt.Errorf("failed to get BMC credentials: %w", err)
	}

	// Connect to the Redfish service using gofish
	client, err := gofish.Connect(gofish.ClientConfig{Endpoint: uri.String(), Username: bmcCreds.Username, Password: bmcCreds.Password, Insecure: q.Insecure})
	if err != nil {
		return fmt.Errorf("failed to connect to Redfish service: %w", err)
	}
	defer client.Logout()

	// Retrieve the UpdateService from the Redfish client
	updateService, err := client.Service.UpdateService()
	if err != nil {
		return fmt.Errorf("failed to get update service: %w", err)
	}

	// Get the update status
	status := updateService.Status
	fmt.Printf("Update Status: %v\n", status)

	return nil
}
