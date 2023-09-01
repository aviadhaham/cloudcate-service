package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func getProfiles() ([]string, error) {
	// read .aws/creds file
	f, err := os.Open(fmt.Sprintf("%s/.aws/credentials", os.Getenv("HOME")))
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()

	profileList := []string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "[") && strings.Contains(line, "]") {
			profile := strings.Trim(scanner.Text(), "[]")
			profileList = append(profileList, profile)
		}
	}

	return profileList, err
}

func getRegions() ([]string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	client := ec2.NewFromConfig(cfg)

	input := &ec2.DescribeRegionsInput{
		AllRegions: aws.Bool(true),
	}

	resp, err := client.DescribeRegions(context.TODO(), input)
	if err != nil {
		log.Fatalf("failed to describe regions, %v", err)
	}

	regionsList := []string{}
	for _, region := range resp.Regions {
		regionsList = append(regionsList, *region.RegionName)
	}

	return regionsList, err
}

func findLoadBalancer(config aws.Config, region string, searchValue string, found *int32) ([]string, error) {
	if atomic.LoadInt32(found) == 1 {
		return nil, nil
	}
	config.Region = region

	elbv2Client := elasticloadbalancingv2.NewFromConfig(config)
	input := &elasticloadbalancingv2.DescribeLoadBalancersInput{}
	output, err := elbv2Client.DescribeLoadBalancers(context.TODO(), input)
	if err != nil && output == nil {
		if strings.Contains(err.Error(), "InvalidClientTokenId") || strings.Contains(err.Error(), "no identity-based policy allows the elasticloadbalancing:DescribeLoadBalancers action") {
			return nil, err
		}
		// fmt.Println("error describing load balancers", err)
	}

	loadBalancers := output.LoadBalancers
	for _, lb := range loadBalancers {
		if strings.Contains(*lb.LoadBalancerArn, searchValue) {
			atomic.StoreInt32(found, 1)
			lbArnSlice := strings.Split(*lb.LoadBalancerArn, ":")
			return lbArnSlice, nil
		}
	}

	return nil, err
}

func getAwsAccount(cfg aws.Config, region string) *string {
	cfg.Region = region

	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		// fmt.Println("Unable to get caller identity:", err)
		return nil
	}
	return identity.Account
}

func findS3Bucket(config aws.Config, region string, searchValue string, found *int32) string {
	if atomic.LoadInt32(found) == 1 {
		return ""
	}

	config.Region = region

	s3Client := s3.NewFromConfig(config)
	output, err := s3Client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		// fmt.Printf("Unable to list buckets, %v", err)
		return nil
	}

	for _, bucket := range output.Buckets {
		if strings.Contains(*bucket.Name, searchValue) {
			atomic.StoreInt32(found, 1)
			return *bucket.Name
		}
	}
	return nil
}

func findResourceInRegion(profile string, cfg aws.Config, region string, resourceType string, resourceName string, found *int32) {
	associatedAwsAccount := getAwsAccount(cfg, region)

	switch resourceType {
	case "loadbalancer":
		lbArnSlice, _ := findLoadBalancer(cfg, region, resourceName, found)
		// if lbArnSlice == nil {
		// 	fmt.Printf("no load balancer was found: %s", resourceName)
		// }
		// if err != nil {
		// 	fmt.Printf("%s", err)
		// }
		if lbArnSlice != nil {
			fmt.Printf("Region: %s\nAWS Account: %s\nLB Details: %s", lbArnSlice[3], lbArnSlice[4], lbArnSlice[5])
		}
	case "s3":
		bucketName := findS3Bucket(cfg, region, resourceName, found)
		if bucketName != "" {
			fmt.Printf("S3 bucket: %s -> AWS account: %s", *bucketName, *associatedAwsAccount)
		}
	}
}

func main() {
	profiles, err := getProfiles()
	if err != nil {
		log.Fatalf("Failed to get profiles: %v", err)
	}

	regions, err := getRegions()
	if err != nil {
		log.Fatalf("Failed to get regions: %v", err)
	}

	// resourceSearchFunctions := map[string]interface{}{
	// 	"LB": findLoadBalancer,
	// }

	resourceName := "amb-aws-config-prod-k8s"
	resourceType := "s3"

	var wg sync.WaitGroup
	var found int32

	for _, profile := range profiles {
		cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithSharedConfigProfile(profile))
		if err != nil {
			log.Fatalf("failed to load configuration for profile, %v", err)
		}

		for _, region := range regions {
			wg.Add(1)
			go func(profile string, cfg aws.Config, region string) {
				defer wg.Done()
				findResourceInRegion(profile, cfg, region, resourceType, resourceName, &found)
			}(profile, cfg, region)
		}
	}
	wg.Wait()
}
