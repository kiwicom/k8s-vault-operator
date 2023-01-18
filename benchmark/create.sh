#!/bin/bash

cat ./token-secret.yaml | sed "s/\$VAULT_TOKEN/$VAULT_TOKEN/g" | kubectl apply -f -

for i in $(seq 1 1000); do 
    echo "---"
    cat vault-secret.yaml | sed "s/\$COUNT/$i/g" | kubectl apply -f -
done
