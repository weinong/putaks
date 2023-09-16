package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
)

var dryRun = flag.Bool("dryRun", false, "Dry Run")
var pemFile = flag.String("pem", "", "PEM File")
var clientID = flag.String("clientID", "", "Client ID")
var csvFile = flag.String("csv", "", "CSV File")

func init() {
	flag.Parse()
}

func main() {
	fmt.Printf("Dry Run: %t\n", *dryRun)
	fmt.Printf("PEM File: %s\n", *pemFile)
	fmt.Printf("Client ID: %s\n", *clientID)
	fmt.Printf("CSV File: %s\n", *csvFile)

	data, err := os.ReadFile(*pemFile)
	if err != nil {
		log.Fatal(err)
	}

	certs, key, err := azidentity.ParseCertificates(data, nil)
	if err != nil {
		log.Fatalf("failed to parse certificate: %v", err)
	}

	f, err := os.Open(*csvFile)
	if err != nil {
		log.Fatal(err)
	}

	r := csv.NewReader(bufio.NewReader(f))
	tenantID := ""

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		subscriptionID, resourceGroup, resourceName := parseResourceID(record[0])
		fmt.Printf("subscriptionID: %s, resourceGroup: %s, resourceName: %s, tenantID: %s\n", subscriptionID, resourceGroup, resourceName, record[1])

		var cred *azidentity.ClientCertificateCredential

		if tenantID != record[1] {
			tenantID = record[1]
			cred, err = azidentity.NewClientCertificateCredential(tenantID, *clientID, certs, key, &azidentity.ClientCertificateCredentialOptions{
				SendCertificateChain:     true,
				DisableInstanceDiscovery: true,
			})
			if err != nil {
				log.Fatalf("failed to create credential: %v", err)
			}
		}

		putMC(cred, subscriptionID, resourceGroup, resourceName)

		break
	}

}

func parseResourceID(resourceID string) (string, string, string) {
	parts := strings.Split(resourceID, "/")
	return parts[2], parts[4], parts[8]
}

func putMC(cred *azidentity.ClientCertificateCredential, subscriptionID, resourceGroup, resourceName string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*3)
	defer cancel()

	clientFactory, err := armcontainerservice.NewClientFactory("", cred, nil)
	if err != nil {
		log.Fatalf("failed to create client factory: %v", err)
	}

	if *dryRun {
		return
	}
	poller, err := clientFactory.NewManagedClustersClient().BeginCreateOrUpdate(ctx, resourceGroup, resourceName, armcontainerservice.ManagedCluster{}, nil)

	if err != nil {
		log.Printf("failed to finish the request: %v\n", err)
	}
	res, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		log.Printf("failed to pull the result: %v\n", err)
	}
	_ = res
}
