// Package main demonstrates deploying google-group-sync as a standalone AWS Lambda
// function using Pulumi Go SDK. The Lambda is deployed with:
// - ARM64 (Graviton) for cost efficiency
// - Lambda Web Adapter (LWA) layer for HTTP translation
// - Function URL with AWS_IAM authentication
// - SA key loaded from Secrets Manager at startup
//
// This is the "standalone Lambda" mode — google-group-sync runs as its own Lambda
// function and is called via Function URL or lambda:InvokeFunction.
//
// For the "extension sidecar" mode (running inside another Lambda), see
// deploy/example-extension/.
//
// Prerequisites:
// - AWS credentials configured
// - Google Workspace service account key JSON stored in Secrets Manager
//
// Usage:
//
//	cd deploy/example-lambda
//	pulumi up
package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	// Lambda Web Adapter layer ARN (arm64, eu-central-1).
	// Find your region's ARN at: https://github.com/awslabs/aws-lambda-web-adapter#lambda-layer
	lwaLayerARN = "arn:aws:lambda:eu-central-1:753240598075:layer:LambdaAdapterLayerArm64:25"

	// Version of google-group-sync to deploy.
	version = "0.4.0"

	// GitHub Release ZIP URL (Lambda binary named "bootstrap").
	zipURL = "https://github.com/truvity/google-group-sync/releases/download/v" + version + "/google-group-sync_lambda_" + version + "_arm64.zip"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		// Configuration values (set via `pulumi config set`).
		googleAdminEmail := cfg.Require("googleAdminEmail")
		googleSAKeyJSON := cfg.RequireSecret("googleSAKeyJSON")

		// IAM role for the Lambda function.
		lambdaRole, err := iam.NewRole(ctx, "google-group-sync-role", &iam.RoleArgs{
			Name: pulumi.String("google-group-sync"),
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Action": "sts:AssumeRole",
					"Effect": "Allow",
					"Principal": {"Service": "lambda.amazonaws.com"}
				}]
			}`),
		})
		if err != nil {
			return fmt.Errorf("create IAM role: %w", err)
		}

		// Attach basic execution policy (CloudWatch Logs).
		_, err = iam.NewRolePolicyAttachment(ctx, "google-group-sync-basic", &iam.RolePolicyAttachmentArgs{
			Role:      lambdaRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		})
		if err != nil {
			return fmt.Errorf("attach basic execution policy: %w", err)
		}

		// Store SA key in Secrets Manager (Lambda reads it at startup via SA_KEY_SECRET_NAME).
		saKeySecret, err := secretsmanager.NewSecret(ctx, "google-sa-key", &secretsmanager.SecretArgs{
			Name:                 pulumi.String("google-group-sync/sa-key"),
			RecoveryWindowInDays: pulumi.Int(0),
		})
		if err != nil {
			return fmt.Errorf("create SA key secret: %w", err)
		}

		_, err = secretsmanager.NewSecretVersion(ctx, "google-sa-key-version", &secretsmanager.SecretVersionArgs{
			SecretId:     saKeySecret.ID(),
			SecretString: googleSAKeyJSON,
		})
		if err != nil {
			return fmt.Errorf("create SA key secret version: %w", err)
		}

		// Secrets Manager read policy for the Lambda function.
		smPolicy, err := iam.NewRolePolicy(ctx, "google-group-sync-sm-read", &iam.RolePolicyArgs{
			Role: lambdaRole.Name,
			Policy: saKeySecret.Arn.ApplyT(func(arn string) string {
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [{
						"Effect": "Allow",
						"Action": ["secretsmanager:GetSecretValue"],
						"Resource": [%q]
					}]
				}`, arn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return fmt.Errorf("create SM read policy: %w", err)
		}

		// CloudWatch Log Group.
		logGroup, err := cloudwatch.NewLogGroup(ctx, "google-group-sync-logs", &cloudwatch.LogGroupArgs{
			Name:            pulumi.String("/aws/lambda/google-group-sync"),
			RetentionInDays: pulumi.Int(7),
		})
		if err != nil {
			return fmt.Errorf("create log group: %w", err)
		}

		// Lambda function.
		fn, err := lambda.NewFunction(ctx, "google-group-sync", &lambda.FunctionArgs{
			Name:    pulumi.String("google-group-sync"),
			Role:    lambdaRole.Arn,
			Runtime: pulumi.String("provided.al2023"),
			Architectures: pulumi.StringArray{
				pulumi.String("arm64"),
			},
			Handler:    pulumi.String("bootstrap"),
			Timeout:    pulumi.Int(30),
			MemorySize: pulumi.Int(128),
			Code:       pulumi.NewRemoteArchive(zipURL),
			Layers: pulumi.StringArray{
				pulumi.String(lwaLayerARN),
			},
			LoggingConfig: &lambda.FunctionLoggingConfigArgs{
				LogGroup:  logGroup.Name,
				LogFormat: pulumi.String("JSON"),
			},
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"PORT":                         pulumi.String("8080"),
					"HEALTH_PORT":                  pulumi.String("7070"),
					"AWS_LWA_READINESS_CHECK_PATH": pulumi.String("/health"),
					"AWS_LWA_READINESS_CHECK_PORT": pulumi.String("7070"),
					"AWS_LAMBDA_EXEC_WRAPPER":      pulumi.String("/opt/bootstrap"),
					"AWS_LWA_ASYNC_INIT":           pulumi.String("true"),
					"SA_KEY_SECRET_NAME":           pulumi.String("google-group-sync/sa-key"),
					"GOOGLE_ADMIN_EMAIL":           pulumi.String(googleAdminEmail),
					"LOG_FORMAT":                   pulumi.String("json"),
					"CACHE_TTL":                    pulumi.String("5m"),
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{logGroup, smPolicy}))
		if err != nil {
			return fmt.Errorf("create Lambda function: %w", err)
		}

		// Function URL with AWS_IAM auth.
		fnURL, err := lambda.NewFunctionUrl(ctx, "google-group-sync-url", &lambda.FunctionUrlArgs{
			FunctionName:      fn.Name,
			AuthorizationType: pulumi.String("AWS_IAM"),
		})
		if err != nil {
			return fmt.Errorf("create Function URL: %w", err)
		}

		// Exports.
		ctx.Export("functionName", fn.Name)
		ctx.Export("functionArn", fn.Arn)
		ctx.Export("functionUrl", fnURL.FunctionUrl)
		ctx.Export("saKeySecretArn", saKeySecret.Arn)

		return nil
	})
}
