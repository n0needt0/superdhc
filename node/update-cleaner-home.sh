NODE=`cat /etc/dhc4/dhc4-name.cfg`
MYIP=`cat /etc/dhc4/dhc4-ip.cfg`

stop cleaner-home

cp /vagrant/etc/dhc4/cleaner-home.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/cleaner-home.cfg
sed -i "s/MYIPADDRESS/$MYIP/g" /etc/dhc4/cleaner-home.cfg

#binary
cp /vagrant/bin/cleaner-home /var/dhc4/cleaner-home
chmod 777 /var/dhc4/cleaner-home

start cleaner-home
