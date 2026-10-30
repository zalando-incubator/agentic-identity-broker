package main

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// Constants for encryption infrastructure configuration.
const (
	// IAM role session duration (hours).
	MaxSessionDurationHours = 1

	// KMS key pending deletion window for production (days, maximum for safety).
	ProdDeletionWindowDays = 30

	// KMS key pending deletion window for non-production (days, minimum for fast cleanup).
	NonProdDeletionWindowDays = 7

	// CloudWatch metrics period (minutes).
	MetricsPeriodMinutes = 5

	// CloudWatch alarm thresholds.
	KMSThrottleThreshold   = 10 // throttled requests over 2 evaluation periods
	KMSThrottleEvalPeriods = 2  // evaluation periods for throttle alarm
	KMSErrorThreshold      = 5  // error count over 1 evaluation period
	KMSErrorEvalPeriods    = 1  // evaluation periods for error alarm

)

// EncryptionStackProps extends StackProps with encryption-specific parameters.
type EncryptionStackProps struct {
	awscdk.StackProps

	// Environment is the deployment environment: "test", "sandbox", or "prod".
	Environment string

	// ServiceAccountSubject is the full OIDC subject claim bound in the trust policy condition.
	// For Kubernetes workloads this is typically "system:serviceaccount:<namespace>:<sa-name>".
	ServiceAccountSubject string

	// OIDCProviderArn is the IAM OIDC identity-provider ARN bound as the trust principal (Federated).
	OIDCProviderArn string

	// OIDCSubjectKey is the IAM condition key for the subject claim, e.g. "<oidc-provider-host>:sub".
	OIDCSubjectKey string

	// Tags is a map of optional resource tags to apply to all taggable resources.
	// Standard tags (Project, Component, Environment) are applied by default.
	// Tags provided here will override defaults or add additional tags (e.g., Application, Team, CostCenter).
	// Example: map[string]string{"Application": "TokenVault", "Team": "Security"}
	Tags map[string]string
}

// NewEncryptionStack creates the Token Vault encryption infrastructure stack.
//
// Resources provisioned:
//   - AWS KMS symmetric CMK (KEK for the hierarchical keyring)
//   - DynamoDB table (branch key cache for the hierarchical keyring)
//   - IAM role with least-privilege KMS + DynamoDB permissions
//
// Stack outputs are named to map directly to the application's IDENTITY_BROKER_*
// environment variables.
//
// Props is required; if nil, the function will panic with a clear error message.
func NewEncryptionStack(scope constructs.Construct, id string, props *EncryptionStackProps) awscdk.Stack {
	if props == nil {
		panic("NewEncryptionStack requires props to be non-nil; provide EncryptionStackProps with Environment and ServiceAccountSubject")
	}

	// Normalize into a local copy so the caller's struct is not mutated.
	normalized := *props
	if normalized.Environment == "production" {
		normalized.Environment = "prod"
	}
	props = &normalized
	isProd := props.Environment == "prod"

	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	// Add the required CloudFormation template Metadata header.
	// CI/CD tooling requires these fields to identify and validate the synthesized
	// CloudFormation template used by deployment pipelines.
	stack.TemplateOptions().SetMetadata(&map[string]interface{}{
		"StackName": stack.StackName(),
		"Tags": map[string]interface{}{
			"application": "agentic-identity-broker",
			"component":   "encryption-vault",
			"environment": props.Environment,
		},
	})

	// ─── Service Account Validation ──────────────────────────────────────
	//
	// ServiceAccountSubject is required for all environments — omitting it
	// leaves the IAM OIDC trust policy without the bound Kubernetes subject.
	if props.ServiceAccountSubject == "" {
		panic(fmt.Sprintf(
			"ERROR: serviceAccountSubject is required for env=%s.\n"+
				"Usage: cdk synth -c env=%s -c serviceAccountSubject=<subject> -c oidcProviderArn=<oidc-provider-arn> -c oidcSubjectKey=<oidc-provider-host>:sub",
			props.Environment, props.Environment))
	}

	if props.OIDCProviderArn == "" || props.OIDCSubjectKey == "" {
		panic(fmt.Sprintf(
			"ERROR: oidcProviderArn and oidcSubjectKey are required for env=%s.\n"+
				"Usage: cdk synth -c env=%s -c serviceAccountSubject=<subject> -c oidcProviderArn=<oidc-provider-arn> -c oidcSubjectKey=<oidc-provider-host>:sub",
			props.Environment, props.Environment))
	}

	// ─── Tags ───────────────────────────────────────────────────────────
	//
	// Apply tags to all taggable resources. Merges default tags with user-provided overrides.
	applyStackTags(stack, props)

	// ─── KMS Key (KEK) ──────────────────────────────────────────────────
	//
	// Symmetric CMK used as the Key Encryption Key in the hierarchical keyring.
	// The AWS Encryption SDK wraps per-operation DEKs with branch keys that are
	// themselves protected by this CMK.
	// KMS key alias uses the "agentic-identity-broker" prefix for consistency with other resources.
	// Keep documentation in sync:
	// - docs/operations/deployment-checklist.md
	// - docs/deployment/kubernetes-irsa.md
	// - adrs/20260217-120000-cdk-encryption-infrastructure.md
	kmsKey := awskms.NewKey(stack, jsii.String("EncryptionKEK"), &awskms.KeyProps{
		Description:       jsii.String(fmt.Sprintf("Agentic Identity Broker - Token Vault KEK (%s)", props.Environment)),
		KeySpec:           awskms.KeySpec_SYMMETRIC_DEFAULT,
		KeyUsage:          awskms.KeyUsage_ENCRYPT_DECRYPT,
		EnableKeyRotation: jsii.Bool(true),
		Alias:             jsii.String(fmt.Sprintf("alias/agentic-identity-broker/%s/token-vault-kek", props.Environment)),
		// Prod: RETAIN on stack deletion to prevent accidental data loss.
		// Non-prod: DESTROY for clean teardown.
		RemovalPolicy: removalPolicy(isProd),
		// 30-day pending deletion window for production (max safety).
		PendingWindow: pendingWindow(isProd),
	})

	// ─── DynamoDB Table (Branch Key Cache) ──────────────────────────────
	//
	// The AWS Encryption SDK Hierarchical Keyring requires a DynamoDB table
	// with the specific schema: branch-key-id (S) + type (S).
	// This is the exact schema expected by the KeyStore client implementation:
	// github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographykeystoresmithygenerated
	// See: https://docs.aws.amazon.com/encryption-sdk/latest/developer-guide/use-hierarchical-keyring.html
	//
	// Resource names use the "AgenticIdentityBroker" prefix to match the full
	// project name. Keep documentation in sync:
	// - docs/operations/deployment-checklist.md
	// - docs/deployment/kubernetes-irsa.md
	// - adrs/20260217-120000-cdk-encryption-infrastructure.md
	tableName := fmt.Sprintf("AgenticIdentityBrokerBranchKeys-%s", props.Environment)
	branchKeyTable := awsdynamodb.NewTable(stack, jsii.String("BranchKeyTable"), &awsdynamodb.TableProps{
		TableName: jsii.String(tableName),
		// Schema required by the AWS Encryption SDK KeyStore.
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("branch-key-id"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("type"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		BillingMode: awsdynamodb.BillingMode_PAY_PER_REQUEST,
		Encryption:  awsdynamodb.TableEncryption_AWS_MANAGED,
		PointInTimeRecoverySpecification: &awsdynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: jsii.Bool(isProd),
		},
		DeletionProtection: jsii.Bool(isProd),
		RemovalPolicy:      removalPolicy(isProd),
	})

	// ─── CloudWatch Alarms (KMS Monitoring) ─────────────────────────────
	//
	// Monitor KMS key health and throttling to ensure encryption operations
	// remain performant and catch quota issues early.

	// KMS Throttle Alarm - alerts when KMS API throttling is detected.
	// Throttling indicates approaching or exceeding KMS request quota.
	kmsThrottleMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/KMS"),
		MetricName: jsii.String("ThrottleCount"),
		DimensionsMap: &map[string]*string{
			"KeyId": kmsKey.KeyId(),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(MetricsPeriodMinutes)),
	})

	kmsThrottleAlarm := awscloudwatch.NewAlarm(stack, jsii.String("KMSThrottleAlarm"), &awscloudwatch.AlarmProps{
		Metric:             kmsThrottleMetric,
		Threshold:          jsii.Number(KMSThrottleThreshold),
		EvaluationPeriods:  jsii.Number(KMSThrottleEvalPeriods),
		AlarmName:          jsii.String(fmt.Sprintf("AgenticIdentityBroker-Encryption-%s-KMS-Throttle", props.Environment)),
		AlarmDescription:   jsii.String("KMS key throttling detected - may indicate insufficient quota"),
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
	})

	// KMS User Errors Alarm - alerts on KMS API errors (access denied, key disabled, etc.).
	// Helps catch permission issues, key state problems, or misconfiguration.
	kmsErrorMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/KMS"),
		MetricName: jsii.String("UserErrorCount"),
		DimensionsMap: &map[string]*string{
			"KeyId": kmsKey.KeyId(),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(MetricsPeriodMinutes)),
	})

	kmsErrorAlarm := awscloudwatch.NewAlarm(stack, jsii.String("KMSErrorAlarm"), &awscloudwatch.AlarmProps{
		Metric:             kmsErrorMetric,
		Threshold:          jsii.Number(KMSErrorThreshold),
		EvaluationPeriods:  jsii.Number(KMSErrorEvalPeriods),
		AlarmName:          jsii.String(fmt.Sprintf("AgenticIdentityBroker-Encryption-%s-KMS-Errors", props.Environment)),
		AlarmDescription:   jsii.String("KMS API errors detected - check key policy and permissions"),
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
	})

	// ─── IAM Role (Encryption Operations) ───────────────────────────────
	//
	// Dedicated role with least-privilege access to KMS and DynamoDB.
	// The trust policy is fully replaced below to bind the IAM OIDC identity provider as
	// the federated principal; the typed CDK API cannot express the dynamic condition key.
	encryptionRole := awsiam.NewRole(stack, jsii.String("EncryptionRole"), &awsiam.RoleProps{
		RoleName:           jsii.String(fmt.Sprintf("AgenticIdentityBrokerEncryptionRole-%s", props.Environment)),
		Description:        jsii.String("IAM role for Agentic Identity Broker encryption operations (KMS + DynamoDB)"),
		AssumedBy:          awsiam.NewAccountRootPrincipal(), // overridden below — without the override this role is assumable by any principal in the account
		MaxSessionDuration: awscdk.Duration_Hours(jsii.Number(MaxSessionDurationHours)),
	})

	// Override the trust policy with a manual IAM OIDC trust relationship.
	// The dynamic StringEquals key is supplied through CDK context and rendered verbatim.
	cfnRole := encryptionRole.Node().DefaultChild().(awsiam.CfnRole)
	cfnRole.AddPropertyOverride(
		jsii.String("AssumeRolePolicyDocument"),
		map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Effect": "Allow",
					"Principal": map[string]interface{}{
						"Federated": props.OIDCProviderArn,
					},
					"Action": "sts:AssumeRoleWithWebIdentity",
					"Condition": map[string]interface{}{
						"StringEquals": map[string]interface{}{
							props.OIDCSubjectKey: props.ServiceAccountSubject,
						},
					},
				},
			},
		},
	)

	// KMS permissions: encrypt, decrypt, generate/re-encrypt data keys.
	// These are the minimum permissions required by the hierarchical keyring.
	encryptionRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Sid:    jsii.String("KMSHierarchicalKeyringOperations"),
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"kms:Encrypt",
			"kms:Decrypt",
			"kms:GenerateDataKey",
			"kms:GenerateDataKeyWithoutPlaintext",
			"kms:ReEncryptFrom",
			"kms:ReEncryptTo",
			"kms:DescribeKey",
		),
		Resources: &[]*string{kmsKey.KeyArn()},
	}))

	// DynamoDB permissions: the KeyStore needs read/write access for branch key CRUD.
	encryptionRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Sid:    jsii.String("DynamoDBBranchKeyOperations"),
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"dynamodb:GetItem",
			"dynamodb:PutItem",
			"dynamodb:Query",
			"dynamodb:UpdateItem",
			"dynamodb:DeleteItem",
			"dynamodb:DescribeTable",
		),
		Resources: &[]*string{branchKeyTable.TableArn()},
	}))

	// KMS:CreateGrant permission — required by the hierarchical keyring
	// to delegate key operations to DynamoDB for branch key wrapping.
	encryptionRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Sid:    jsii.String("KMSCreateGrantForKeyring"),
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"kms:CreateGrant",
		),
		Resources: &[]*string{kmsKey.KeyArn()},
		Conditions: &map[string]interface{}{
			"Bool": map[string]*string{
				"kms:GrantIsForAWSResource": jsii.String("true"),
			},
		},
	}))

	// ─── Stack Outputs ──────────────────────────────────────────────────
	//
	// These map directly to the IDENTITY_BROKER_* environment variables
	// consumed by the application's EncryptionConfig.
	//
	// To inject into the app:
	//   export IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN=$(aws cloudformation describe-stacks \
	//     --stack-name <stack> --query "Stacks[0].Outputs[?OutputKey=='EncryptionKeyARN'].OutputValue" --output text)

	awscdk.NewCfnOutput(stack, jsii.String("EncryptionKeyARN"), &awscdk.CfnOutputProps{
		Value:       kmsKey.KeyArn(),
		Description: jsii.String("KMS CMK ARN → IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN"),
		ExportName:  jsii.String(fmt.Sprintf("AgenticIdentityBroker-%s-EncryptionKeyARN", props.Environment)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("EncryptionKeyAlias"), &awscdk.CfnOutputProps{
		Value:       jsii.String(fmt.Sprintf("alias/agentic-identity-broker/%s/token-vault-kek", props.Environment)),
		Description: jsii.String("KMS CMK Alias for human-readable reference"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("BranchKeyTableName"), &awscdk.CfnOutputProps{
		Value:       branchKeyTable.TableName(),
		Description: jsii.String("DynamoDB table name → IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME"),
		ExportName:  jsii.String(fmt.Sprintf("AgenticIdentityBroker-%s-BranchKeyTableName", props.Environment)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("BranchKeyTableARN"), &awscdk.CfnOutputProps{
		Value:       branchKeyTable.TableArn(),
		Description: jsii.String("DynamoDB table ARN (for IAM policy reference)"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("EncryptionRoleARN"), &awscdk.CfnOutputProps{
		Value:       encryptionRole.RoleArn(),
		Description: jsii.String("IAM role ARN (for reference)"),
		ExportName:  jsii.String(fmt.Sprintf("AgenticIdentityBroker-%s-EncryptionRoleARN", props.Environment)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("KMSThrottleAlarmArn"), &awscdk.CfnOutputProps{
		Value:       kmsThrottleAlarm.AlarmArn(),
		Description: jsii.String("CloudWatch Alarm ARN for KMS throttling (subscribe SNS topic for alerts)"),
		ExportName:  jsii.String(fmt.Sprintf("AgenticIdentityBroker-%s-KMSThrottleAlarmArn", props.Environment)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("KMSErrorAlarmArn"), &awscdk.CfnOutputProps{
		Value:       kmsErrorAlarm.AlarmArn(),
		Description: jsii.String("CloudWatch Alarm ARN for KMS errors (subscribe SNS topic for alerts)"),
		ExportName:  jsii.String(fmt.Sprintf("AgenticIdentityBroker-%s-KMSErrorAlarmArn", props.Environment)),
	})

	// ─── IAM Role Output ─────────────────────────────────────────────────
	awscdk.NewCfnOutput(stack, jsii.String("IamRoleName"), &awscdk.CfnOutputProps{
		Value:       encryptionRole.RoleName(),
		Description: jsii.String("IAM role name for iam.amazonaws.com/role annotation in Kubernetes ServiceAccount"),
		ExportName:  jsii.String(fmt.Sprintf("AgenticIdentityBroker-%s-IamRoleName", props.Environment)),
	})

	// ─── CloudWatch Dashboard ───────────────────────────────────────────
	//
	// Create operational dashboard for encryption infrastructure monitoring.
	createDashboard(stack, kmsKey, branchKeyTable, props)

	return stack
}

// removalPolicy returns RETAIN for production, DESTROY for non-production.
func removalPolicy(isProd bool) awscdk.RemovalPolicy {
	if isProd {
		return awscdk.RemovalPolicy_RETAIN
	}
	return awscdk.RemovalPolicy_DESTROY
}

// pendingWindow returns the KMS key pending deletion window.
// Production: 30 days (maximum, for safety). Non-production: 7 days (minimum, for fast cleanup).
func pendingWindow(isProd bool) awscdk.Duration {
	if isProd {
		return awscdk.Duration_Days(jsii.Number(ProdDeletionWindowDays))
	}
	return awscdk.Duration_Days(jsii.Number(NonProdDeletionWindowDays))
}

// applyStackTags applies default and custom tags to all stack resources.
// Default tags are always applied.
// Environment is always set to match props.Environment.
// User-provided Tags in props override or extend the defaults.
func applyStackTags(stack awscdk.Stack, props *EncryptionStackProps) {
	allTags := map[string]string{
		"application": "agentic-identity-broker",
		"component":   "encryption-vault",
	}
	for key, value := range props.Tags {
		allTags[key] = value
	}
	allTags["environment"] = props.Environment // always overrides user-provided value
	for key, value := range allTags {
		awscdk.Tags_Of(stack).Add(jsii.String(key), jsii.String(value), nil)
	}
}

// createDashboard creates a CloudWatch dashboard for encryption infrastructure monitoring.
// Displays KMS API metrics and DynamoDB branch key cache utilization for operational visibility.
//
// Metrics displayed:
//   - KMS ApiCalls (Sum): Number of KMS API calls for encryption/decryption
//   - KMS ThrottleCount (Sum): Throttled KMS API requests
//   - DynamoDB ConsumedReadCapacityUnits (Sum): Branch key cache read operations
//   - DynamoDB ConsumedWriteCapacityUnits (Sum): Branch key cache write operations
//
// Operators can reproduce these metrics via AWS CLI using metric names above.
// See: docs/operations/deployment-checklist.md for example commands.
func createDashboard(stack awscdk.Stack, kmsKey awskms.IKey, table awsdynamodb.ITable, props *EncryptionStackProps) awscloudwatch.Dashboard {
	dashboard := awscloudwatch.NewDashboard(stack, jsii.String("EncryptionDashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("AgenticIdentityBroker-Encryption-%s", props.Environment)),
	})

	// KMS API Calls Metric - tracks encryption/decryption operations.
	kmsApiCallsMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/KMS"),
		MetricName: jsii.String("ApiCalls"),
		DimensionsMap: &map[string]*string{
			"KeyId": kmsKey.KeyId(),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(MetricsPeriodMinutes)),
	})

	// KMS Throttled Requests Metric - monitors throttling issues.
	kmsThrottleMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/KMS"),
		MetricName: jsii.String("ThrottleCount"),
		DimensionsMap: &map[string]*string{
			"KeyId": kmsKey.KeyId(),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(MetricsPeriodMinutes)),
	})

	// DynamoDB Read Capacity Metric - tracks branch key cache reads.
	dynamoReadMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/DynamoDB"),
		MetricName: jsii.String("ConsumedReadCapacityUnits"),
		DimensionsMap: &map[string]*string{
			"TableName": table.TableName(),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(MetricsPeriodMinutes)),
	})

	// DynamoDB Write Capacity Metric - tracks branch key cache writes.
	dynamoWriteMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/DynamoDB"),
		MetricName: jsii.String("ConsumedWriteCapacityUnits"),
		DimensionsMap: &map[string]*string{
			"TableName": table.TableName(),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(MetricsPeriodMinutes)),
	})

	// Add widgets to dashboard - two columns layout.
	dashboard.AddWidgets(
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title: jsii.String("KMS API Operations"),
			Left: &[]awscloudwatch.IMetric{
				kmsApiCallsMetric,
				kmsThrottleMetric,
			},
			Width: jsii.Number(12),
		}),
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title: jsii.String("DynamoDB Branch Key Cache"),
			Left: &[]awscloudwatch.IMetric{
				dynamoReadMetric,
				dynamoWriteMetric,
			},
			Width: jsii.Number(12),
		}),
	)

	return dashboard
}
