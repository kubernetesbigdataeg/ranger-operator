#!/bin/bash
source /opt/keedio/var/lib/ranger/ranger.env
cd /ranger/dist
python -m SimpleHTTPServer 8080 &
/opt/keedio/bin/render.py -t /opt/keedio/var/lib/ranger/solrconfig.xml.jinja2 -o /opt/keedio/var/lib/ranger/solr_config/solrconfig.xml
/opt/keedio/bin/wait_and_init_zk.sh
/opt/keedio/bin/wait_and_init_solr.sh
/opt/keedio/bin/render.py -t /opt/keedio/var/lib/ranger/ranger_usersync.jinja2 -o /ranger/ranger-usersync/install.properties
/opt/keedio/bin/render.py -t /opt/keedio/var/lib/ranger/ranger.jinja2 -o /ranger/ranger-admin/install.properties
cd /ranger/ranger-admin
./setup.sh 
cd /ranger/ranger-usersync
./setup.sh
ranger-admin start
ranger-usersync start
sleep 10
tail -f /ranger/ranger-admin/ews/logs/* /ranger/ranger-usersync/logs/*
