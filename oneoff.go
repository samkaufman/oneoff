package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/ec2"
)

func main() {

	// TODO: Parse flags
	//   --help describing AWS_REGION and key env vars
	//   --region (default us-east-1)
	//   --availability-zone (default us-east-1a)
	//   --ami-id (default ami-1ecae776)
	//   --shutdown-only
	//   --working-dir for the directory to upload and in which to exec.
	//   --no-auto-init to avoid auto-guessing and installing deps
	//   --instance-type (default: t2.micro)
	//   --key-name (default: none; can't SSH in)
	//   --update-pkgs (default: no; corresponds to repo_update and repo_upgrade in cloud-config)

	// Build the user data for our new instance, which will
	// be the script to launch our process manager.
	userData := `#cloud-config
repo_update: false
#repo_upgrade: __ (all)

packages:
 - golang

runcmd:
 - halt`

	// Create an EC2 service object in the "us-east-1" region
	// Note that you can also configure your region globally by
	// exporting the AWS_REGION environment variable
	region := os.Getenv("AWS_REGION")
	if len(region) == 0 {
		region = "us-east-1"
	}
	svc := ec2.New(&aws.Config{
		Credentials:             aws.DefaultChainCredentials,
		Endpoint:                "",
		Region:                  region,
		DisableSSL:              false,
		ManualSend:              false,
		HTTPClient:              http.DefaultClient,
		LogHTTPBody:             false,
		LogLevel:                0,
		Logger:                  os.Stdout,
		MaxRetries:              aws.DEFAULT_RETRIES,
		DisableParamValidation:  false,
		DisableComputeChecksums: false,
		S3ForcePathStyle:        false,
	})

	// Spin up an instance.
	reservation, err := svc.RunInstances(&ec2.RunInstancesInput{
		ImageID:                           aws.String("ami-1ccae774"),
		MinCount:                          aws.Long(1),
		MaxCount:                          aws.Long(1),
		InstanceType:                      aws.String("t1.micro"),
		InstanceInitiatedShutdownBehavior: aws.String("terminate"),
		UserData: aws.String(base64.StdEncoding.EncodeToString([]byte(userData))),
	})
	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		BailWithAWSError(awserr)
	} else if err != nil {
		// A non-service error occurred.
		panic(err)
	}

	// Select one instance of interest. If there are many,
	// just pick one, but warn the user.
	var instance *ec2.Instance
	if len(reservation.Instances) == 0 {
		panic("No instances in returned Reservation")
	} else if len(reservation.Instances) == 1 {
		instance = reservation.Instances[0]
	} else {
		instance = reservation.Instances[0]
		fmt.Errorf("Multiple instances returned; selecting one arbitrarily")
	}

	// Wait for status to be running.
	PollForRunning(svc, *instance.InstanceID)

	// TODO: Okay, now how do we get both live tailing
	// and (rotating!) logs which are reliably pushed to S3?
}

/**
 * Return when image reports status is "running".
 * Doesn't time out.
 */
func pollForRunning(svc *ec2.EC2, instanceID string) {

	params := ec2.DescribeInstancesInput{
		InstanceIDs: []*string{aws.String(instanceID)},
	}

	for {
		// Do the network call
		output, err := svc.DescribeInstances(&params)
		if awserr := aws.Error(err); awserr != nil {
			// A service error occurred.
			BailWithAWSError(awserr)
		} else if err != nil {
			// A non-service error occurred.
			panic(err)
		}
		if len(output.Reservations) != 1 {
			panic("unexpected number of reservations in DescribeInstances reply")
		}
		if len(output.Reservations[0].Instances) != 1 {
			panic("unexpected number of instances in DescribeInstances reply")
		}

		// Return if the instance is ready, otherwise sleep
		// and check again.
		statusName := output.Reservations[0].Instances[0].State.Name
		if *statusName == "running" {
			return
		}

		time.Sleep(800 * time.Millisecond)
	}
}

/**
 * Handles printing the right error message
 * and exiting the process in the event of an
 * AWS error.
 *
 * TODO: Seriously rethink cleaning up after errors.
 */
func bailWithAWSError(awserr *aws.APIError) {
	fmt.Println("AWS Error:", awserr.Code, awserr.Message)
	fmt.Println("")
	fmt.Println("To be safe, you should confirm that there are no")
	fmt.Println("zombie EC2 instances running.")
	fmt.Println("")
	os.Exit(1)
}
