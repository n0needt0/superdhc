NODE=`cat /etc/dhc4/dhc4-name.cfg`
MYIP=`cat /etc/dhc4/dhc4-ip.cfg`

stop judge-home

cp /vagrant/etc/dhc4/judge-home.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/judge-home.cfg
sed -i "s/MYIPADDRESS/$MYIP/g" /etc/dhc4/judge-home.cfg

#binary
cp /vagrant/bin/judge-home /var/dhc4/judge-home
chmod 777 /var/dhc4/judge-home

start judge-home
