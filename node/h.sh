

/sbin/stop home-cleaner

/sbin/stop home-node

#INSTALL HOME-NODE
#home-node config
cp /vagrant/etc/fortihealth/home-node.cfg /etc/fortihealth/
sed -i 's/THISNODEID/home1/g' /etc/fortihealth/home-node.cfg

#home-node binary
cp /vagrant/bin/home-node /var/fortihealth/home-node
chmod 777 /var/fortihealth/home-node

#home-node monit and init files
cp /vagrant/etc/init/home-node.conf /etc/init/
cp /vagrant/etc/monit/conf.d/home-node /etc/monit/conf.d/

/sbin/stop home-node
/sbin/start home-node

#INSTALL HOME-CLEANER
#node config
cp /vagrant/etc/fortihealth/home-cleaner.cfg /etc/fortihealth/
sed -i 's/THISNODEID/home1/g' /etc/fortihealth/home-cleaner.cfg

#home-cleaner binary
cp /vagrant/bin/home-cleaner /var/fortihealth/home-cleaner
chmod 777 /var/fortihealth/home-cleaner

#node monit and init files
cp /vagrant/etc/init/home-cleaner.conf /etc/init/
cp /vagrant/etc/monit/conf.d/home-cleaner /etc/monit/conf.d/

/sbin/stop home-cleaner
/sbin/start home-cleaner
