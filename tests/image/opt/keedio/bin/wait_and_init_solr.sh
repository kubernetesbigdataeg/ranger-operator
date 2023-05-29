#!/bin/bash 
source /opt/keedio/var/lib/ranger/ranger.env
SOLR_HOST=`echo ${SOLR_HOST}|cut -d',' -f1`
while ! nc -z $SOLR_HOST $SOLR_PORT;
do
  echo Waiting SOLR;
  sleep 1;
done
set -xv
curl "${SOLR_HOST}:${SOLR_PORT}/solr/admin/collections?action=CREATE&name=${SOLR_COLLECTION_NAME}&numShards=${SOLR_NO_SHARD}&replicationFactor=${SOLR_NO_REPLICA}&collection.configName=${SOLR_CONFIG_NAME}&maxShardsPerNode=100"
