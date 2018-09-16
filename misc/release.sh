#!/bin/bash
if [ "$1" = "" ]
then
  echo "Usage: $0 <version>"
  exit
fi

mkdir -p BUILDS
mkdir -p BUILDS/$1
rm -rf BUILDS/$1/*

gomobile bind -target=ios github.com/centrifugal/centrifuge-mobile
gomobile bind -target=android github.com/centrifugal/centrifuge-mobile

zip -r BUILDS/$1/centrifuge-mobile-ios-$1.zip Centrifuge.framework/
zip -r BUILDS/$1/centrifuge-mobile-android-$1.zip centrifuge.aar centrifuge-sources.jar
