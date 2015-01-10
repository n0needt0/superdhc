#!/usr/bin/env bash

# Get root up in here
sudo su

export DEBIAN_FRONTEND=noninteractive
export NODE=home3
export MYIP=192.168.82.120

#create log directory
mkdir -p /var/log/dhc4
chmod 777 /var/log/dhc4

#create dhc4 config dir
mkdir -p /etc/dhc4
chmod 777 /etc/dhc4

echo "$NODE" > /etc/dhc4/dhc4-name.cfg
echo "$MYIP" > /etc/dhc4/dhc4-ip.cfg

#create dhc4 binary dir
mkdir -p /var/dhc4
chmod 777 /var/dhc4

#install log rotate
cp /vagrant/etc/logrotate.d/dhc4  /etc/logrotate.d/

#INSTALL HOME-CLEANER
#config
cp /vagrant/etc/dhc4/cleaner-home.cfg /etc/dhc4/
sed -i 's/THISNODEID/$NODE/g' /etc/dhc4/cleaner-home.cfg
sed -i 's/MYIPADDRESS/$MYIP/g' /etc/dhc4/cleaner-home.cfg

#binary
cp /vagrant/bin/cleaner-home /var/dhc4/cleaner-home
chmod 777 /var/dhc4/cleaner-home

#monit and init files
cp /vagrant/etc/init/cleaner-home.conf /etc/init/
cp /vagrant/etc/monit/conf.d/cleaner-home /etc/monit/conf.d/

/sbin/stop cleaner-home
/sbin/start cleaner-home
#END INSTALL HOME-CLEANER

#INSTALL HOME-JUDGE
#config
cp /vagrant/etc/dhc4/judge-home.cfg /etc/dhc4/
sed -i 's/THISNODEID/$NODE/g' /etc/dhc4/judge-home.cfg
sed -i 's/MYIPADDRESS/$MYIP/g' /etc/dhc4/judge-home.cfg

#binary
cp /vagrant/bin/judge-home /var/dhc4/judge-home
chmod 777 /var/dhc4/judge-home

#monit and init files
cp /vagrant/etc/init/judge-home.conf /etc/init/
cp /vagrant/etc/monit/conf.d/judge-home /etc/monit/conf.d/

/sbin/stop judge-home
/sbin/start judge-home
#END INSTALL HOME-JUDGE

#INSTALL HOME-HISTORY
#config
cp /vagrant/etc/dhc4/history-home.cfg /etc/dhc4/
sed -i 's/THISNODEID/$NODE/g' /etc/dhc4/history-home.cfg
sed -i 's/MYIPADDRESS/$MYIP/g' /etc/dhc4/history-home.cfg

#binary
cp /vagrant/bin/history-home /var/dhc4/history-home
chmod 777 /var/dhc4/history-home

#monit and init files
cp /vagrant/etc/init/history-home.conf /etc/init/
cp /vagrant/etc/monit/conf.d/history-home /etc/monit/conf.d/

/sbin/stop history-home
/sbin/start history-home
#END INSTALL HOME-HISTORY

cp /vagrant/etc/ganglia/gmond_node.conf /etc/ganglia/gmond.conf
sed -i "s/THISNODEID/$NODE/g" /etc/ganglia/gmond.conf

#install MongoDb Ganglia Support
 mkdir /usr/lib/ganglia/python_modules

 cp /vagrant/usr/lib/ganglia/python_modules/mongodb.py  /usr/lib/ganglia/python_modules/

 mkdir /etc/ganglia/conf.d
 cp /vagrant/etc/ganglia/conf.d/* /etc/ganglia/conf.d/

/etc/init.d/ganglia-monitor restart
echo "done!"
