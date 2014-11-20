#!/usr/bin/env bash

# Get root up in here
sudo su

export DEBIAN_FRONTEND=noninteractive

# Update and begin installing some utility tools
apt-get -y update
apt-get install -y python-software-properties
apt-get install -y git curl
apt-get install -y build-essential

#installing regular 3.1.7 repo ganglia NOT latest as it breaks shit up
apt-get update

#create stupid dirs otherwise it breaks
mkdir -p /usr/share/ganglia-webfrontend/lib/dwoo/cache
mkdir -p /usr/share/ganglia-webfrontend/lib/dwoo/compiled

chown www-data /usr/share/ganglia-webfrontend/lib/dwoo/cache
chown www-data /usr/share/ganglia-webfrontend/lib/dwoo/compiled

apt-get install rrdtool gmetad ganglia-webfrontend ganglia-monitor -y

#using this repo to install ganglia-monitor 3.4 as it allows for server name overwrites
add-apt-repository ppa:rufustfirefly/ganglia

apt-get update

apt-get install ganglia-monitor -y

cp /vagrant/etc/ganglia/gmetad.conf /etc/ganglia/
cp /vagrant/etc/ganglia/gmond_server.conf /etc/ganglia/gmond.conf
sed -i 's/THISNODEID/monitor/g' /etc/ganglia/gmond.conf

echo "Alias /ganglia /usr/share/ganglia-webfrontend" >> /etc/apache2/apache2.conf

/etc/init.d/ganglia-monitor restart
/etc/init.d/gmetad restart
/etc/init.d/apache2 restart

echo "done!"
