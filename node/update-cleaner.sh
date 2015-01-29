NODE=`cat /etc/dhc4/dhc4-name.cfg`

stop cleaner

cp /vagrant/bin/cleaner /var/dhc4
cp /vagrant/etc/dhc4/cleaner.cfg /etc/dhc4

sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/cleaner.cfg

start cleaner
