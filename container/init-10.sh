#!/bin/bash
echo "Checking for database connection..."
while ! mysqladmin ping -h mariadb -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" --silent; do
    echo "Database not ready, waiting..."
    sleep 3
done
echo "Database is running and accepting connections."

REPOSDIR=`pwd`
if [ ! -f $REPOSDIR/civicrm-version.txt ]; then
  REPOSDIR='/mnt/neticrm-10/civicrm'
fi

export DRUPAL=10
export DRUPAL_ROOT=/var/www/html

echo "export TERM=xterm" >> /root/.bashrc
echo "export DRUPAL_ROOT=/var/www/html" >> /root/.bashrc

date +"@ %Y-%m-%d %H:%M:%S %z"
echo "CI for Drupal-$DRUPAL + netiCRM"

if mysql -h mariadb -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "SHOW DATABASES;" > /dev/null 2>&1; then
    echo "✅ Connection successful!"
else
    echo "❌ Connection failed!"
    echo "Error message:"
    mysql -h mariadb -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "SHOW DATABASES;" 2>&1
fi

cd $DRUPAL_ROOT
echo "Install Drupal ..."
date +"@ %Y-%m-%d %H:%M:%S %z"
sleep 5s
drush -vv --yes site-install standard --account-name=admin --db-url=mysql://$MYSQL_USER:${MYSQL_PASSWORD}@mariadb/$MYSQL_DATABASE --account-pass=$MYSQL_PASSWORD --site-name=netiCRM

if [ -f $DRUPAL_ROOT/sites/default/settings.php ]; then
  echo 'date_default_timezone_set("Asia/Taipei");' >> $DRUPAL_ROOT/sites/default/settings.php
  echo 'ini_set("error_reporting", E_ALL & ~E_NOTICE & ~E_STRICT & ~E_DEPRECATED & ~E_WARNING);' >> $DRUPAL_ROOT/sites/default/settings.php
  echo "\$base_url='';" >> $DRUPAL_ROOT/sites/default/settings.php
  echo "\$settings['civicrm_demo.sample_data_ci'] = TRUE;" >> $DRUPAL_ROOT/sites/default/settings.php
  echo "\$config['system.performance']['js']['preprocess'] = FALSE;" >> $DRUPAL_ROOT/sites/default/settings.php
fi

ln -s /mnt/neticrm-10/civicrm $DRUPAL_ROOT/modules/civicrm

drush --yes pm:install civicrm
drush --yes pm:install civicrm_allpay
drush --yes pm:install civicrm_spgateway
drush --yes pm:install neticrm_drush
drush --yes pm:install civicrm_demo

# add permission for unit testing
drush role-add-perm anonymous 'profile create,register for events,access CiviMail subscribe/unsubscribe pages,access all custom data,view event info,view public CiviMail content,make online contributions'
drush role-add-perm authenticated 'profile create,register for events,access CiviMail subscribe/unsubscribe pages,access all custom data,view event info,view public CiviMail content,make online contributions,profile edit'

# add user login block to front page
mkdir /tmp/config
printf "langcode: en\nstatus: true\ndependencies:\n  module:\n    - user\n  theme:\n    - olivero\nid: userlogin\ntheme: olivero\nregion: sidebar\nweight: 0\nprovider: null\nplugin: user_login_block\nsettings:\n  id: user_login_block\n  label: 'User login'\n  label_display: visible\n  provider: user\nvisibility: {  }" > /tmp/config/block.block.userlogin.yml
printf "langcode: en\nstatus: true\ndependencies:\n  module:\n    - image\n    - user\nid: user.user.default\ntargetEntityType: user\nbundle: user\nmode: default\ncontent:\n  member_for:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 1\n    region: content\n  user_picture:\n    type: image\n    label: hidden\n    settings:\n      image_link: content\n      image_style: thumbnail\n      image_loading:\n        attribute: lazy\n    third_party_settings: {  }\n    weight: 0\n    region: content\n  civicrm_dashboard:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 3\n    region: content\n  civicrm_profiles:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 4\n    region: content\n  civicrm_record:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 2\n    region: content\nhidden:\n\n" > /tmp/config/core.entity_view_display.user.user.default.yml
drush --yes config:import --source=/tmp/config --partial

chown -R www-data /var/www/html/sites/default/files

drush runserver 0.0.0.0:$PORT >& /dev/null & 
until netstat -an 2>/dev/null | grep "${PORT}.*LISTEN"; do true; done