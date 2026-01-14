#!/usr/bin/env bash
#
# verify-custom-files-v2.sh - Production-ready verification script
# Verifies that custom obot-entraid files are preserved after upstream merge
#
# Usage: ./scripts/verify-custom-files-v2.sh [OPTIONS]
#
# Options:
#   --help, -h          Show this help message
#   --no-color          Disable color output
#   --json              Output results in JSON format
#   --verbose, -v       Enable verbose output
#   --quiet, -q         Suppress non-error output
#   --version           Show script version
#
# Exit Codes:
#   0 - All checks passed (or only warnings)
#   1 - Verification failed (missing files or errors)
#   2 - Invalid usage or options
#

set -Eeuo pipefail
shopt -s inherit_errexit 2>/dev/null || true

# Script metadata
readonly SCRIPT_VERSION="2.0.0"
readonly SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

# Configuration
USE_COLOR=true
OUTPUT_FORMAT="text"
VERBOSE=false
QUIET=false

# Track verification status
ERRORS=0
WARNINGS=0
CHECKS_PASSED=0
CHECKS_TOTAL=0

# Results tracking for JSON output
declare -a CHECK_RESULTS=()

# Color codes (will be disabled if USE_COLOR=false)
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Error handler
error_handler() {
    local line_no=$1
    local exit_code=$2
    printf "Error: Script failed at line %d with exit code %d\n" "$line_no" "$exit_code" >&2
    exit 1
}

trap 'error_handler ${LINENO} $?' ERR

# Cleanup handler
cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]] && [[ "${OUTPUT_FORMAT}" != "json" ]]; then
        printf "\n%bScript terminated unexpectedly%b\n" "$RED" "$NC" >&2
    fi
}

trap cleanup EXIT

# Usage information
show_usage() {
    cat << 'EOF'
Usage: verify-custom-files-v2.sh [OPTIONS]

Verifies that custom obot-entraid files are preserved after upstream merge.
Checks authentication providers, tool registry, CI/CD workflows, and documentation.

Options:
  --help, -h          Show this help message
  --no-color          Disable color output (for CI/CD)
  --json              Output results in JSON format
  --verbose, -v       Enable verbose output (show all checks)
  --quiet, -q         Suppress non-error output
  --version           Show script version

Exit Codes:
  0 - All checks passed (or only warnings)
  1 - Verification failed (missing/corrupted files)
  2 - Invalid usage or options

Examples:
  # Basic usage
  ./scripts/verify-custom-files-v2.sh

  # CI/CD usage
  ./scripts/verify-custom-files-v2.sh --no-color --json > results.json

  # Verbose output
  ./scripts/verify-custom-files-v2.sh --verbose

Repository: https://github.com/jrmatherly/obot-entraid
EOF
}

# Version information
show_version() {
    printf "%s version %s\n" "$SCRIPT_NAME" "$SCRIPT_VERSION"
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --help|-h)
                show_usage
                exit 0
                ;;
            --version)
                show_version
                exit 0
                ;;
            --no-color)
                USE_COLOR=false
                shift
                ;;
            --json)
                OUTPUT_FORMAT="json"
                USE_COLOR=false  # Disable colors in JSON mode
                shift
                ;;
            --verbose|-v)
                VERBOSE=true
                shift
                ;;
            --quiet|-q)
                QUIET=true
                shift
                ;;
            *)
                printf "Error: Unknown option: %s\n" "$1" >&2
                printf "Run '%s --help' for usage information.\n" "$SCRIPT_NAME" >&2
                exit 2
                ;;
        esac
    done

    # Validate conflicting options
    if [[ "$VERBOSE" == true ]] && [[ "$QUIET" == true ]]; then
        printf "Error: Cannot use --verbose and --quiet together\n" >&2
        exit 2
    fi

    # Auto-detect TTY for color output
    if [[ "$USE_COLOR" == true ]] && [[ ! -t 1 ]]; then
        USE_COLOR=false
    fi

    # Disable colors if requested
    if [[ "$USE_COLOR" == false ]]; then
        RED=''
        YELLOW=''
        GREEN=''
        BLUE=''
        NC=''
    fi
}

# Logging functions
log_verbose() {
    if [[ "$VERBOSE" == true ]] && [[ "$OUTPUT_FORMAT" != "json" ]]; then
        printf "%b" "$*"
    fi
}

log_info() {
    if [[ "$QUIET" == false ]] && [[ "$OUTPUT_FORMAT" != "json" ]]; then
        printf "%b" "$*"
    fi
}

log_section() {
    if [[ "$QUIET" == false ]] && [[ "$OUTPUT_FORMAT" != "json" ]]; then
        printf "\n%b=== %s ===%b\n" "$BLUE" "$1" "$NC"
    fi
}

# Safe counter increments
increment_errors() {
    ERRORS=$((ERRORS + 1))
}

increment_warnings() {
    WARNINGS=$((WARNINGS + 1))
}

increment_checks_passed() {
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
}

increment_checks_total() {
    CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
}

# Add check result for JSON output
add_check_result() {
    local status=$1
    local type=$2
    local file=$3
    local description=$4
    local details=${5:-""}

    if [[ "$OUTPUT_FORMAT" == "json" ]]; then
        local result="{\"status\":\"$status\",\"type\":\"$type\",\"file\":\"$file\",\"description\":\"$description\""
        if [[ -n "$details" ]]; then
            result="$result,\"details\":\"$details\""
        fi
        result="$result}"
        CHECK_RESULTS+=("$result")
    fi
}

# Check if file exists
check_file() {
    local file=$1
    local description=$2

    increment_checks_total

    log_verbose "Checking file: %s\n" "$file"

    if [[ -f "$file" ]]; then
        log_info "%b✓%b %s: %s\n" "$GREEN" "$NC" "$description" "$file"
        increment_checks_passed
        add_check_result "pass" "file_exists" "$file" "$description"
        return 0
    else
        log_info "%b✗%b %s: %s %bMISSING%b\n" "$RED" "$NC" "$description" "$file" "$RED" "$NC"
        increment_errors
        add_check_result "error" "file_exists" "$file" "$description" "File does not exist"
        return 1
    fi
}

# Check if file exists and is not empty
check_file_not_empty() {
    local file=$1
    local description=$2

    increment_checks_total

    log_verbose "Checking file (non-empty): %s\n" "$file"

    if [[ ! -e "$file" ]]; then
        log_info "%b✗%b %s: %s %bMISSING%b\n" "$RED" "$NC" "$description" "$file" "$RED" "$NC"
        increment_errors
        add_check_result "error" "file_integrity" "$file" "$description" "File does not exist"
        return 1
    elif [[ ! -s "$file" ]]; then
        log_info "%b✗%b %s: %s %bEMPTY OR ZERO SIZE%b\n" "$RED" "$NC" "$description" "$file" "$RED" "$NC"
        increment_errors
        add_check_result "error" "file_integrity" "$file" "$description" "File is empty or has zero size"
        return 1
    else
        log_info "%b✓%b %s: %s\n" "$GREEN" "$NC" "$description" "$file"
        increment_checks_passed
        add_check_result "pass" "file_integrity" "$file" "$description"
        return 0
    fi
}

# Check if file is executable
check_file_executable() {
    local file=$1
    local description=$2

    increment_checks_total

    log_verbose "Checking file (executable): %s\n" "$file"

    if [[ ! -e "$file" ]]; then
        log_info "%b✗%b %s: %s %bMISSING%b\n" "$RED" "$NC" "$description" "$file" "$RED" "$NC"
        increment_errors
        add_check_result "error" "file_executable" "$file" "$description" "File does not exist"
        return 1
    elif [[ ! -x "$file" ]]; then
        log_info "%b⚠%b %s: %s %bNOT EXECUTABLE%b\n" "$YELLOW" "$NC" "$description" "$file" "$YELLOW" "$NC"
        increment_warnings
        add_check_result "warning" "file_executable" "$file" "$description" "File exists but is not executable"
        return 1
    else
        log_info "%b✓%b %s: %s\n" "$GREEN" "$NC" "$description" "$file"
        increment_checks_passed
        add_check_result "pass" "file_executable" "$file" "$description"
        return 0
    fi
}

# Check if directory exists
check_directory() {
    local dir=$1
    local description=$2

    increment_checks_total

    log_verbose "Checking directory: %s\n" "$dir"

    if [[ -d "$dir" ]]; then
        log_info "%b✓%b %s: %s\n" "$GREEN" "$NC" "$description" "$dir"
        increment_checks_passed
        add_check_result "pass" "directory_exists" "$dir" "$description"
        return 0
    else
        log_info "%b✗%b %s: %s %bMISSING%b\n" "$RED" "$NC" "$description" "$dir" "$RED" "$NC"
        increment_errors
        add_check_result "error" "directory_exists" "$dir" "$description" "Directory does not exist"
        return 1
    fi
}

# Check file content with pattern matching
check_content() {
    local file=$1
    local pattern=$2
    local description=$3
    local case_insensitive=${4:-false}

    increment_checks_total

    log_verbose "Checking content in %s for pattern: %s\n" "$file" "$pattern"

    if [[ ! -f "$file" ]]; then
        log_info "%b✗%b Cannot check %s: file %s does not exist\n" "$RED" "$NC" "$description" "$file"
        increment_errors
        add_check_result "error" "content_check" "$file" "$description" "File does not exist"
        return 1
    fi

    local grep_flags="-q"
    if [[ "$case_insensitive" == true ]]; then
        grep_flags="-iq"
    fi

    if grep $grep_flags -E "$pattern" "$file" 2>/dev/null; then
        log_info "%b✓%b %s\n" "$GREEN" "$NC" "$description"
        increment_checks_passed
        add_check_result "pass" "content_check" "$file" "$description"
        return 0
    else
        log_info "%b⚠%b %s %bNOT FOUND%b\n" "$YELLOW" "$NC" "$description" "$YELLOW" "$NC"
        increment_warnings
        add_check_result "warning" "content_check" "$file" "$description" "Pattern not found in file"
        return 1
    fi
}

# Validate working directory
validate_working_directory() {
    local project_root
    project_root="$(cd "$SCRIPT_DIR/.." && pwd -P)"

    log_verbose "Validating working directory: %s\n" "$project_root"

    # Check if we're in the project root
    if [[ ! -f "$project_root/go.mod" ]]; then
        printf "%bError: Not in obot-entraid project root%b\n" "$RED" "$NC" >&2
        printf "Expected to find go.mod in: %s\n" "$project_root" >&2
        exit 1
    fi

    # Check if it's a git repository
    if [[ ! -d "$project_root/.git" ]]; then
        log_info "%b⚠%b Warning: Not a git repository\n" "$YELLOW" "$NC"
    fi

    # Change to project root
    cd "$project_root" || exit 1

    log_verbose "Working directory validated: %s\n" "$project_root"
}

# Output results in JSON format
output_json() {
    local status
    if [[ $ERRORS -eq 0 ]]; then
        status="success"
    else
        status="failure"
    fi

    cat << EOF
{
  "version": "$SCRIPT_VERSION",
  "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "status": "$status",
  "summary": {
    "total_checks": $CHECKS_TOTAL,
    "passed": $CHECKS_PASSED,
    "errors": $ERRORS,
    "warnings": $WARNINGS
  },
  "checks": [
$(IFS=,; printf "    %s" "${CHECK_RESULTS[*]}")
  ]
}
EOF
}

# Main verification logic
run_verification() {
    log_info "%b🔍 Verifying custom obot-entraid files...%b\n" "$BLUE" "$NC"

    # Custom Authentication Providers
    log_section "Custom Authentication Providers"
    check_directory "tools/entra-auth-provider" "Entra ID auth provider"
    check_file_not_empty "tools/entra-auth-provider/main.go" "Entra ID main.go"
    check_file "tools/entra-auth-provider/tool.gpt" "Entra ID tool.gpt"
    check_file "tools/entra-auth-provider/go.mod" "Entra ID go.mod"

    check_directory "tools/keycloak-auth-provider" "Keycloak auth provider"
    check_file_not_empty "tools/keycloak-auth-provider/main.go" "Keycloak main.go"
    check_file "tools/keycloak-auth-provider/tool.gpt" "Keycloak tool.gpt"
    check_file "tools/keycloak-auth-provider/go.mod" "Keycloak go.mod"

    check_directory "tools/auth-providers-common" "Common auth provider utilities"
    check_directory "tools/placeholder-credential" "Placeholder credential tool"

    # Custom Tool Registry
    log_section "Custom Tool Registry"
    check_file_not_empty "tools/index.yaml" "Custom tool registry"
    check_content "tools/index.yaml" "entra-auth-provider" "Tool registry contains entra-auth-provider"
    check_content "tools/index.yaml" "keycloak-auth-provider" "Tool registry contains keycloak-auth-provider"

    # Build Infrastructure
    log_section "Build Infrastructure"
    check_file_not_empty "Dockerfile" "Custom Dockerfile"
    # Fixed pattern: case-insensitive match for "merge" + "index.yaml"
    check_content "Dockerfile" "merge.*index\\.ya?ml" "Dockerfile contains registry merge logic" true

    # CI/CD Workflows
    log_section "CI/CD Workflows"
    check_file "scripts/verify-custom-files.sh" "Original verification script"
    check_file_executable "scripts/verify-custom-files.sh" "Verification script executable"

    check_file ".github/workflows/docker-build-and-push.yml" "Docker build workflow"
    check_content ".github/workflows/docker-build-and-push.yml" "runs-on:.*ubuntu-latest" "Workflow uses ubuntu-latest (not depot)"
    check_content ".github/workflows/docker-build-and-push.yml" "ghcr\\.io" "Workflow publishes to GHCR"

    check_file ".github/workflows/helm.yml" "Helm chart workflow"
    check_content ".github/workflows/helm.yml" "ghcr\\.io" "Helm workflow publishes to GHCR"

    check_file ".github/workflows/upstream-sync-check.yml" "Upstream sync check workflow"
    check_file ".github/workflows/test-upstream-merge.yml" "Test upstream merge workflow"

    if [[ -f ".github/workflows/release.yml" ]]; then
        check_file ".github/workflows/release.yml" "Release workflow"
    fi

    # Helm Chart Customizations
    log_section "Helm Chart Customizations"
    check_file_not_empty "chart/Chart.yaml" "Helm chart definition"
    check_file_not_empty "chart/values.yaml" "Helm chart values"
    check_file "chart/templates/deployment.yaml" "Deployment template"

    # Documentation
    log_section "Documentation"
    check_file_not_empty "SECURITY.md" "Security policy (fork-customized)"
    check_content "SECURITY.md" "Auth Provider Security" "SECURITY.md contains fork-specific auth provider security documentation"
    check_file "tools/README.md" "Auth provider documentation"
    check_file "docs/docs/contributing/upstream-merge-process.md" "Upstream merge process"
    check_file "docs/docs/contributing/fork-workflow-analysis-2026.md" "Fork workflow analysis"
    check_file "CONTRIBUTING.md" "Root-level contributing guide"

    if [[ -d "tools/keycloak-auth-provider" ]] && [[ -f "tools/keycloak-auth-provider/KEYCLOAK_SETUP.md" ]]; then
        check_file "tools/keycloak-auth-provider/KEYCLOAK_SETUP.md" "Keycloak setup guide"
    fi

    # Additional integrity checks
    log_section "Additional Integrity Checks"

    # Check Go module files are not empty
    if [[ -f "go.mod" ]]; then
        check_file_not_empty "go.mod" "Root go.mod"
    fi

    if [[ -f "go.sum" ]]; then
        check_file_not_empty "go.sum" "Root go.sum"
    fi

    # Check key source directories exist
    if [[ -d "pkg" ]]; then
        check_directory "pkg" "Core package directory"
    fi

    if [[ -d "ui/user" ]]; then
        check_directory "ui/user" "UI user directory"
    fi

    # Fork-specific UI customizations
    log_section "Fork-Specific UI Customizations"
    check_file "ui/user/src/routes/terms-of-service/+page.svelte" "Terms of Service page"
    check_content "ui/user/src/routes/terms-of-service/+page.svelte" "github\\.com/jrmatherly/obot-entraid" "Terms of Service contains fork GitHub URL"

    check_file "ui/user/src/routes/privacy-policy/+page.svelte" "Privacy Policy page"
    check_content "ui/user/src/routes/privacy-policy/+page.svelte" "github\\.com/jrmatherly/obot-entraid" "Privacy Policy contains fork GitHub URL"
}

# Display summary
display_summary() {
    if [[ "$OUTPUT_FORMAT" == "json" ]]; then
        output_json
        return
    fi

    printf "\n%b=== Summary ===%b\n" "$BLUE" "$NC"
    printf "Total checks: %d\n" "$CHECKS_TOTAL"
    printf "Passed: %b%d%b\n" "$GREEN" "$CHECKS_PASSED" "$NC"
    printf "Errors: %b%d%b\n" "$RED" "$ERRORS" "$NC"
    printf "Warnings: %b%d%b\n" "$YELLOW" "$WARNINGS" "$NC"

    if [[ $ERRORS -eq 0 ]] && [[ $WARNINGS -eq 0 ]]; then
        printf "\n%b✓ All custom files verified successfully!%b\n" "$GREEN" "$NC"
        return 0
    elif [[ $ERRORS -eq 0 ]]; then
        printf "\n%b⚠ Verification completed with %d warning(s)%b\n" "$YELLOW" "$WARNINGS" "$NC"
        printf "Warnings indicate potential issues but are not critical.\n"
        return 0
    else
        printf "\n%b✗ Verification failed with %d error(s) and %d warning(s)%b\n" "$RED" "$ERRORS" "$WARNINGS" "$NC"
        printf "\nThis indicates that some custom obot-entraid files may have been\n"
        printf "lost during the upstream merge. Please review the merge carefully\n"
        printf "and restore any missing custom files before committing.\n"
        return 1
    fi
}

# Main execution
main() {
    parse_args "$@"
    validate_working_directory
    run_verification
    display_summary
}

# Run main function
main "$@"
