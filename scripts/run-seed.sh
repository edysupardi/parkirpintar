#!/usr/bin/env bash
# Seed 400 parking spots into Aurora via ECS run-task.
# Usage: ./run-seed.sh

set -euo pipefail

REGION="ap-southeast-1"
PROJECT="edysup-parkirpintar"
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"
IMAGE="${ECR}/${PROJECT}-seed:latest"
CLUSTER="${PROJECT}-dev-cluster"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Seeding parking spots ==="

# Get DB info
DB_HOST=$(aws rds describe-db-clusters --db-cluster-identifier "${PROJECT}-dev-aurora" --region "$REGION" --query 'DBClusters[0].Endpoint' --output text)
DB_PASS=$(aws secretsmanager get-secret-value --secret-id "${PROJECT}/dev/db-password" --region "$REGION" --query SecretString --output text)

# Get network config
SUBNETS=$(aws ecs describe-services --cluster "$CLUSTER" --services "${PROJECT}-dev-gateway" --region "$REGION" --query 'services[0].networkConfiguration.awsvpcConfiguration.subnets' --output json)
SG=$(aws ecs describe-services --cluster "$CLUSTER" --services "${PROJECT}-dev-gateway" --region "$REGION" --query 'services[0].networkConfiguration.awsvpcConfiguration.securityGroups[0]' --output text)
EXEC_ROLE=$(aws ecs describe-task-definition --task-definition "${PROJECT}-dev-gateway" --region "$REGION" --query 'taskDefinition.executionRoleArn' --output text)

# Create ECR repo if not exists
aws ecr describe-repositories --repository-names "${PROJECT}-seed" --region "$REGION" > /dev/null 2>&1 || \
  aws ecr create-repository --repository-name "${PROJECT}-seed" --region "$REGION" > /dev/null

# Build and push
echo "Building seed image..."
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$ECR" > /dev/null 2>&1
docker build --platform linux/amd64 -f "$ROOT_DIR/scripts/seed/Dockerfile" -t "$IMAGE" "$ROOT_DIR" > /dev/null 2>&1
docker push "$IMAGE" > /dev/null 2>&1
echo "  ✓ Image pushed"

# Register task definition
TASK_DEF=$(cat <<EOF
{
  "family": "${PROJECT}-dev-seed",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "256",
  "memory": "512",
  "executionRoleArn": "${EXEC_ROLE}",
  "containerDefinitions": [{
    "name": "seed",
    "image": "${IMAGE}",
    "essential": true,
    "environment": [
      {"name": "DB_HOST", "value": "${DB_HOST}"},
      {"name": "DB_PORT", "value": "5432"},
      {"name": "DB_USER", "value": "parkirpintar"},
      {"name": "DB_PASSWORD", "value": "${DB_PASS}"},
      {"name": "DB_NAME", "value": "parkirpintar"},
      {"name": "DB_SSL_MODE", "value": "require"}
    ],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-group": "/ecs/${PROJECT}-dev/seed",
        "awslogs-region": "${REGION}",
        "awslogs-stream-prefix": "seed"
      }
    }
  }]
}
EOF
)

# Create log group
aws logs create-log-group --log-group-name "/ecs/${PROJECT}-dev/seed" --region "$REGION" 2>/dev/null || true

echo "Registering task definition..."
TASK_ARN=$(aws ecs register-task-definition --cli-input-json "$TASK_DEF" --region "$REGION" --query 'taskDefinition.taskDefinitionArn' --output text)

# Run the task
echo "Running seed task..."
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

aws ecs wait tasks-stopped --cluster "$CLUSTER" --tasks "$TASK_ID" --region "$REGION"

EXIT_CODE=$(aws ecs describe-tasks --cluster "$CLUSTER" --tasks "$TASK_ID" --region "$REGION" --query 'tasks[0].containers[0].exitCode' --output text)

if [ "$EXIT_CODE" = "0" ]; then
  echo "  ✓ Seeded 400 spots (5 floors × 30 cars + 50 motorcycles)"
else
  echo "  ✗ Seed failed (exit code: $EXIT_CODE)"
  echo "  Check logs:"
  echo "  aws logs get-log-events --log-group-name /ecs/${PROJECT}-dev/seed --log-stream-name \$(aws logs describe-log-streams --log-group-name /ecs/${PROJECT}-dev/seed --order-by LastEventTime --descending --limit 1 --region $REGION --query 'logStreams[0].logStreamName' --output text) --region $REGION --query 'events[].message' --output text"
  exit 1
fi
