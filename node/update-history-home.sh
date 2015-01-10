NODE=`cat /etc/dhc4/dhc4-name.cfg`
MYIP=`cat /etc/dhc4/dhc4-ip.cfg`


stop history-home

cp /vagrant/etc/dhc4/history-home.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/history-home.cfg
sed -i "s/MYIPADDRESS/$MYIP/g" /etc/dhc4/history-home.cfg

#binary
cp /vagrant/bin/history-home /var/dhc4/history-home
chmod 777 /var/dhc4/history-home

start history-home
