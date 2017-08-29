#!/usr/bin/env bash


curl http://localhost:9080/config



curl http://localhost:9080/git/status



curl http://localhost:9080/config --data \
'{"exportPath":"autosave-am","gitBranch":"autosave-test"}'

# switch branches

curl --data 'branch=test' http://localhost:9080/git/branch

# List branches

curl http://localhost:9080/git/branch


# reset
curl --data 'branch=test' http://localhost:9080/git/reset


#todo

link to gitRepo for info purposes
branch should show in all views


