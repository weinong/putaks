package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"io"
	"log"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
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
	log.Printf("Dry Run: %t\n", *dryRun)
	log.Printf("PEM File: %s\n", *pemFile)
	log.Printf("Client ID: %s\n", *clientID)
	log.Printf("CSV File: %s\n", *csvFile)

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

	var cred *azidentity.ClientCertificateCredential

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		subscriptionID, resourceGroup, resourceName := parseResourceID(record[0])
		log.Printf("subscriptionID: %s, resourceGroup: %s, resourceName: %s, tenantID: %s\n", subscriptionID, resourceGroup, resourceName, record[1])

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
	}

}

func parseResourceID(resourceID string) (string, string, string) {
	parts := strings.Split(resourceID, "/")
	return parts[2], parts[4], parts[8]
}

func putMC(cred *azidentity.ClientCertificateCredential, subscriptionID, resourceGroup, resourceName string) {
	clientFactory, err := armcontainerservice.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		log.Fatalf("failed to create client factory: %v", err)
	}

	client := clientFactory.NewManagedClustersClient()

	mc, err := client.Get(context.Background(), resourceGroup, resourceName, nil)
	if err != nil {
		log.Printf("failed to get the cluster: %v\n", err)
		return
	}

	if *dryRun {
		return
	}

	if mc.Identity != nil && mc.Identity.Type != nil && *mc.Identity.Type == armcontainerservice.ResourceIdentityTypeSystemAssigned {
		log.Printf("existing principalID: %s\n", *mc.Identity.PrincipalID)

		_, err = clientFactory.NewManagedClustersClient().BeginCreateOrUpdate(context.Background(), resourceGroup, resourceName, armcontainerservice.ManagedCluster{
			Location: mc.Location,
			Identity: &armcontainerservice.ManagedClusterIdentity{
				Type: to.Ptr(armcontainerservice.ResourceIdentityTypeSystemAssigned),
			},
			SKU:  mc.SKU,
			Tags: mc.Tags,
			Properties: &armcontainerservice.ManagedClusterProperties{
				DNSPrefix: mc.Properties.DNSPrefix,
			},
		}, nil)

		if err != nil {
			log.Printf("failed to finish the request: %v\n", err)
		}

	}

	// don't wait
	//res, err := poller.PollUntilDone(ctx, nil)
	//if err != nil {
	//	log.Printf("failed to pull the result: %v\n", err)
	//}
	//_ = res
}
