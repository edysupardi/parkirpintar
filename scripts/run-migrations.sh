#!/usr/bin/env bash
# Run database migrations against Aurora via ECS run-task.
# Usage: ./run-migrations.sh [up|down|version]
#
# This builds a migration container, pushes to ECR, and runs it
# as a one-off Fargate task in the same VPC as Aurora.

set -euo pipefail

COMMAND="${1:-up}"
REGION="ap-southeast-1"
PROJECT="edysup-parkirpintar"
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"
IMAGE="${ECR}/${PROJECT}-migrate:latest"
CLUSTER="${PROJECT}-dev-cluster"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Running migrations ($COMMAND) ==="

# Get DB endpoint and password
DB_HOST=$(aws rds describe-db-clusters --db-cluster-identifier "${PROJECT}-dev-aurora" --region "$REGION" --query 'DBClusters[0].Endpoint' --output text)
DB_PASS=$(aws secretsmanager get-secret-value --secret-id "${PROJECT}/dev/db-password" --region "$REGION" --query SecretString --output text)
DB_URL="postgres://parkirpintar:${DB_PASS}@${DB_HOST}:5432/parkirpintar?sslmode=require"

# Get network config from ECS service
SUBNETS=$(aws ecs describe-services --cluster "$CLUSTER" --services "${PROJECT}-dev-gateway" --region "$REGION" --query 'services[0].networkConfiguration.awsvpcConfiguration.subnets' --output json)
SG=$(aws ecs describe-services --cluster "$CLUSTER" --services "${PROJECT}-dev-gateway" --region "$REGION" --query 'services[0].networkConfiguration.awsvpcConfiguration.securityGroups[0]' --output text)
EXEC_ROLE=$(aws ecs describe-task-definition --task-definition "${PROJECT}-dev-gateway" --region "$REGION" --query 'taskDefinition.executionRoleArn' --output text)

# Create ECR repo if not exists
aws ecr describe-repositories --repository-names "${PROJECT}-migrate" --region "$REGION" > /dev/null 2>&1 || \
  aws ecr create-repository --repository-name "${PROJECT}-migrate" --region "$REGION" > /dev/null

# Build and push migration image
echo "Building migration image..."
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$ECR" > /dev/null 2>&1
docker build --platform linux/amd64 -t "$IMAGE" "$ROOT_DIR/migrations" > /dev/null 2>&1
docker push "$IMAGE" > /dev/null 2>&1
echo "  ✓ Image pushed"

# Register one-off task definition
TASK_DEF=$(cat <<EOF
{
  "family": "${PROJECT}-dev-migrate",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "256",
  "memory": "512",
  "executionRoleArn": "${EXEC_ROLE}",
  "containerDefinitions": [{
    "name": "migrate",
    "image": "${IMAGE}",
    "essential": true,
    "command": ["${DB_URL}", "${COMMAND}"],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-group": "/ecs/${PROJECT}-dev/migrate",
        "awslogs-region": "${REGION}",
        "awslogs-stream-prefix": "migrate",
        "awslogs-create-group": "true"
      }
    }
  }]
}
EOF
)

echo "Registering task definition..."
TASK_ARN=$(aws ecs register-task-definition --cli-input-json "$TASK_DEF" --region "$REGION" --query 'taskDefinition.taskDefinitionArn' --output text)

# Run the task
echo "Running migration task..."
SUBNET_LIST=$(echo "$SUBNETS" | jq -r 'join(",")')
TASK_ID=$(aws ecs run-task \
  --cluster "$CLUSTER" \
  --task-definition "$TASK_ARN" \
  --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[${SUBNET_LIST}],securityGroups=[${SG}]}" \
  --region "$REGION" \
  --query 'tasks[0].taskArn' --output text)

echo "  Task: $TASK_ID"
echo "  Waiting for completion..."

# Wait for task to stop
aws ecs wait tasks-stopped --cluster "$CLUSTER" --tasks "$TASK_ID" --region "$REGION"

# Check exit code
EXIT_CODE=$(aws ecs describe-tasks --cluster "$CLUSTER" --tasks "$TASK_ID" --region "$REGION" --query 'tasks[0].containers[0].exitCode' --output text)

if [ "$EXIT_CODE" = "0" ]; then
  echo "  ✓ Migration $COMMAND completed successfully"
else
  echo "  ✗ Migration failed (exit code: $EXIT_CODE)"
  echo "  Check logs: aws logs get-log-events --log-group-name /ecs/${PROJECT}-dev/migrate --log-stream-name \$(aws logs describe-log-streams --log-group-name /ecs/${PROJECT}-dev/migrate --order-by LastEventTime --descending --limit 1 --region $REGION --query 'logStreams[0].logStreamName' --output text) --region $REGION --query 'events[].message' --output text"
  exit 1
fi
