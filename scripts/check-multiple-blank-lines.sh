#!/bin/bash

# Check for multiple consecutive blank lines in Go source files

set -e

# Find all Go files
GO_FILES=$(find . -name "*.go" -type f | grep -v vendor/ | grep -v .git/)

# Track if any issues were found
FOUND_ISSUES=0

# Function to check a single file
check_file() {
    local file="$1"
    
    # Check for multiple consecutive blank lines (3 or more newlines in a row)
    if grep -Pzo '\n\n\n+' "$file" > /dev/null 2>&1; then
        echo "Error: Multiple consecutive blank lines found in $file"
        
        # Show line numbers where multiple blank lines occur
        awk 'BEGIN{blank=0; start=0} 
             /^$/ {blank++; if(blank==1) start=NR} 
             !/^$/ {if(blank>1) print "  Lines " start "-" (NR-1) ": " blank " consecutive blank lines"; blank=0} 
             END{if(blank>1) print "  Lines " start "-" NR ": " blank " consecutive blank lines at end of file"}' "$file"
        
        FOUND_ISSUES=1
    fi
}

echo "Checking for multiple consecutive blank lines in Go source files..."

# Check each Go file
for file in $GO_FILES; do
    check_file "$file"
done

if [ $FOUND_ISSUES -eq 0 ]; then
    echo "✓ No multiple consecutive blank lines found"
    exit 0
else
    echo ""
    echo "Please remove extra blank lines. Only single blank lines are allowed between functions and code blocks."
    exit 1
fi