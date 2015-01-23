#!/usr/bin/env bash

# Get root up in here
sudo su

export DEBIAN_FRONTEND=noninteractive
export NODE=redis
export MYIP=192.168.82.130

#create log directory
mkdir -p /var/log/dhc4
chmod 777 /var/log/dhc4

#create dhc4 config dir
mkdir -p /etc/dhc4
chmod 777 /etc/dhc4

echo "$NODE" > /etc/dhc4/dhc4-name.cfg
echo "$MYIP" > /etc/dhc4/dhc4-ip.cfg

apt-get update

apt-get install php5-fpm php5-mysql php5-curl php5-gd php5-intl php-pear php5-imagick php5-imap php5-mcrypt php5-memcache php5-ming php5-ps php5-pspell php5-recode php-apc php5-snmp php5-sqlite php5-tidy php5-xmlrpc php5-xsl php5-dev -y
apt-get install redis-server pkg-config -y
sudo pecl install zmq-beta -y

cp -r /vagrant/DHC3 /var

#install log rotate
cp /vagrant/etc/logrotate.d/dhc4  /etc/logrotate.d/

cp /vagrant/etc/ganglia/gmond_node.conf /etc/ganglia/gmond.conf
sed -i "s/THISNODEID/$NODE/g" /etc/ganglia/gmond.conf

#install MongoDb Ganglia Support
 mkdir /usr/lib/ganglia/python_modules

 cp /vagrant/usr/lib/ganglia/python_modules/*  /usr/lib/ganglia/python_modules/

 mkdir /etc/ganglia/conf.d
 cp /vagrant/etc/ganglia/conf.d/* /etc/ganglia/conf.d/

service redis-server restart

/etc/init.d/ganglia-monitor restart
echo "done!"
