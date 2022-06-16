# mediathek

![Logo](./img/logo.png)

A little daemon which downloads the latest videos from [https://mediathekviewweb.de](https://mediathekviewweb.de).

## usage

```bash
# just list the videos
mediathek -query tatort

# download all matches
mediathek -query tatort -download

# run in servermode and fetch the latest videos every 120s
mediathek -query tatort -server -interval 120
```
