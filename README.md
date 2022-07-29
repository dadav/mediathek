# mediathek

<img src="./img/logo.png" width="300">

A little daemon which downloads the latest videos from [https://mediathekviewweb.de](https://mediathekviewweb.de).


## usage

```bash
# just list the videos
mediathek -query tatort

# list different videos
mediathek -query "tatort | sendung mit der maus"

# download all matches
mediathek -query tatort -download

# run in servermode and fetch the latest videos every 120s
mediathek -query tatort -server -interval 120
```
