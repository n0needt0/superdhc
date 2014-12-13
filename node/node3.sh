#!/usr/bin/env bash

# Get root up in here
sudo su

export DEBIAN_FRONTEND=noninteractive
export NODE=node3

#create log directory
mkdir -p /var/log/dhc4
chmod 777 /var/log/dhc4

#create dhc4 config dir
mkdir -p /etc/dhc4
chmod 777 /etc/dhc4

echo "$NODE" > /etc/dhc4/dhc4.cfg

#create dhc4 binary dir
mkdir -p /var/dhc4
chmod 777 /var/dhc4

#install log rotate
cp /vagrant/etc/logrotate.d/dhc4  /etc/logrotate.d/

#INSTALL CLEANER
#cleaner config
cp /vagrant/etc/dhc4/cleaner.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/cleaner.cfg

sed -i 's/MYTARGETS/tcp:\/\/192.168.82.120:6455,tcp:\/\/192.168.82.110:6455,tcp:\/\/192.168.82.100:6455/g' /etc/dhc4/cleaner.cfg

#cleaner binary
cp /vagrant/bin/cleaner /var/dhc4/cleaner
chmod 777 /var/dhc4/cleaner

#cleaner monit and init files
cp /vagrant/etc/init/cleaner.conf /etc/init/
cp /vagrant/etc/monit/conf.d/cleaner /etc/monit/conf.d/

/etc/init.d/mongod restart

/sbin/stop cleaner
/sbin/start cleaner


#INSTALL dispatch
#dispatch config
cp /vagrant/etc/dhc4/dispatch.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/dispatch.cfg

#dispatch binary
cp /vagrant/bin/dispatch /var/dhc4/dispatch
chmod 777 /var/dhc4/dispatch

#dispatch monit and init files
cp /vagrant/etc/init/dispatch.conf /etc/init/
cp /vagrant/etc/monit/conf.d/dispatch /etc/monit/conf.d/

/sbin/stop dispatch
/sbin/start dispatch

#INSTALL NODE
#node config
cp /vagrant/etc/dhc4/node.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/node.cfg

#node binary
cp /vagrant/bin/node /var/dhc4/node
chmod 777 /var/dhc4/node

#node monit and init files
cp /vagrant/etc/init/node.conf /etc/init/
cp /vagrant/etc/monit/conf.d/node /etc/monit/conf.d/

/sbin/stop node
/sbin/start node

cp /vagrant/etc/ganglia/gmond_node.conf /etc/ganglia/gmond.conf
sed -i "s/THISNODEID/$NODE/g" /etc/ganglia/gmond.conf

#install MongoDb Ganglia Support
 mkdir /usr/lib/ganglia/python_modules

 cp /vagrant/usr/lib/ganglia/python_modules/mongodb.py  /usr/lib/ganglia/python_modules/

 mkdir /etc/ganglia/conf.d
 cp /vagrant/etc/ganglia/conf.d/* /etc/ganglia/conf.d/

/etc/init.d/ganglia-monitor restart

mongo --host 192.168.42.100 << 'EOF'
config = { _id: "rs0", members:[
          { _id : 0, host : "192.168.42.100:27017"},
          { _id : 1, host : "192.168.42.110:27017"},
          { _id : 2, host : "192.168.42.120:27017"} ]
         };
rs.initiate(config);

rs.status();

EOF

/etc/init.d/mongod restart

###FAKEDATA INSERT####
echo "sleeping  30sec for mongo to catchup"

sleep 30

mongo --host 192.168.42.100 << 'EOF'

rs.status();

EOF

mongo 192.168.42.100:27017/hc /vagrant/node_fakejson.js

#TODO
stop cleaner

echo "done!"

