#!/bin/bash

USER=$1
DB=$2
TABLE=$3
BATCH_SIZE=10
BUFFER=""
COUNT=0

while IFS= read -r line; do
    # Append the line to the buffer with a newline
    BUFFER+="$line"$'\n'
    ((COUNT++))
    if ((COUNT % BATCH_SIZE == 0)); then
        # Remove trailing newline before sending
        BUFFER="${BUFFER%$'\n'}"
        psql -U "$USER" -d "$DB" >& error.log <<EOF
\COPY $TABLE FROM STDIN WITH CSV
$BUFFER
EOF
        BUFFER=""
    fi
done

# Process remaining lines in the buffer
if [[ -n "$BUFFER" ]]; then
    psql -U "$USER" -d "$DB" <<EOF
\COPY $TABLE FROM STDIN WITH CSV
$BUFFER
EOF
fi
