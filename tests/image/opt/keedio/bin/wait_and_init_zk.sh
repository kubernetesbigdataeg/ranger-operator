#!/bin/bash
source /opt/keedio/var/lib/ranger/ranger.env
ZOOKEEPER_HOST=$(echo $ZOOKEEPERS|cut -d',' -f1 | cut -d':' -f1)
ZOOKEEPER_PORT=$(echo $ZOOKEEPERS|cut -d',' -f1 | cut -d':' -f2)
ZKCLI="/zookeeper/bin/zkCli.sh -server ${ZOOKEEPERS}"
ZK_STATUS=`echo ruok|nc $ZOOKEEPER_HOST $ZOOKEEPER_PORT`
TRY=0
while [ "$ZK_STATUS" != "imok" ];
do
  ((TRY++))
  if [ $TRY -ge 10 ];then
    echo "Zookeeper not available"
    exit 1
  fi
  echo Waiting zk
  sleep 10
  ZK_STATUS=`echo ruok|nc $ZOOKEEPER_HOST $ZOOKEEPER_PORT`
done

TRY=0
$ZKCLI get ${ZNODE}/configs/${SOLR_CONFIG_NAME}
RC=$?
while [ "$RC" != 0 ];
do
  $ZKCLI create ${ZNODE}/configs/${SOLR_CONFIG_NAME}
  RC=$?
  ((TRY++));
  if [ $TRY -ge 10 ];then 
    exit 1
  else
    sleep 1
  fi
done
for i in `ls /opt/keedio/var/lib/ranger/solr_config/*`;do
  TRY=0
  RC=1
  while [ $RC != 0 ]; do
    $ZKCLI create ${ZNODE}/configs/${SOLR_CONFIG_NAME}/$(basename $i) "`cat $i`"
    RC=$?
    ((TRY++))
    if [ $TRY -ge 10 ];then 
      exit 1
    else
      sleep 1
    fi
  done
done
