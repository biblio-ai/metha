# Generate sites.tsv OAI endpoint list from various other sources.
#
# requires: python, curl, pup, jq, moreutils
#
SHELL := /bin/bash

sites.tsv: sites-extra.tsv sites-roar.tsv sites-oa.tsv
	cat $^ | python unique_by_schema.py | sort -u > $@
	# try to filter out predatory journals
	grep -v -f <(curl -sL "https://scholarlyoa.com/list-of-standalone-journals/" | pup 'li > a[href] json{}' | jq -rc '.[].href' | grep -Ev "(aiac.org.au|sfu.ca|scholarlyoa|google.com)" | cut -d / -f 3) sites.tsv | sponge sites.tsv
	grep -v -f <(curl -sL "https://predatoryjournals.com/publishers/" | pup 'li > a[href] json{}' | jq -rc '.[].href' | grep "^http" | grep -vE "(github|twitter)" | cut -d / -f 3) sites.tsv | sponge sites.tsv
	grep -v "web.archive.org/web" sites.tsv | sponge sites.tsv
	sort -o sites.tsv -u sites.tsv


.PHONY: clean
clean:
	rm -f sites.tsv
	rm -rf .ipynb_checkpoints/

.PHONY: ping.ndj
ping.ndj:
	clinker -verbose -w 228 < sites.tsv > ping.ndj.tmp && mv ping.ndj.tmp ping.ndj

predatory.tsv:
	curl -sL "https://scholarlyoa.com/list-of-standalone-journals/" | pup 'li > a[href] json{}' | jq -rc '.[].href' | grep -Ev "(aiac.org.au|sfu.ca|scholarlyoa|google.com)" | cut -d / -f 3 > predatory.tsv
	curl -sL "https://predatoryjournals.com/publishers/" | pup 'li > a[href] json{}' | jq -rc '.[].href' | grep "^http" | grep -vE "(github|twitter)" | cut -d / -f 3 >> predatory.tsv
	sort -u predatory.tsv | sponge predatory.tsv

