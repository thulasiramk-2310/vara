# Working with other people

VARA does not have a hosted service like GitHub. To work with friends, teammates or
colleagues, **one of you runs a server** (the Hub) and everyone else uses the `vara`
command to clone, push and pull. This page shows how, from the smallest setup to a
shared server that stays on.

Everything here was run end to end on one computer with two accounts (clone, commit,
push and pull in both directions, refusing someone who has no access, and taking access
away again). It has not yet been tried across a real network with HTTPS, so treat the
network parts as a guide and tell us what you find.

## The idea in four steps

1. **One person runs the server.** It keeps the repositories and the list of accounts.
2. **They make an account for each person** and **share** the repository with them.
3. **Everyone installs `vara`**, signs in once, and clones.
4. **Everyone pushes and pulls** like in any other version control. Merge conflicts can be
   settled by choice (`vara merge --help`).

## 1. Run a server

Pick the setup that fits how you work.

### A. Everyone in the same room (a lab, a classroom, a phone or laptop hotspot)

On the computer that will hold the repositories:

```sh
mkdir -p ~/hub/{repos,policy,meta,accounts}

# the first account: the owner of the server (you)
vara account create --accounts ~/hub/accounts --policy ~/hub/policy \
  --username ada --password 'choose-a-strong-password'

# let other computers on the network connect
vara serve --addr 0.0.0.0:8737 \
  --root ~/hub/repos --accounts ~/hub/accounts \
  --policy ~/hub/policy --meta ~/hub/meta --hub ~/hub/hub-ui
```

`--hub` is optional: it is the folder of the web interface (download
`vara-hub-ui_<version>.tar.gz` from the release page and unpack it). Without it, cloning,
pushing and pulling all work; only the web pages are missing.

Then tell your teammates your computer's address on that network, for example
`http://192.168.1.20:8737`. On Linux, `ip addr` shows it.

**This is plain HTTP.** Passwords and code cross the network unencrypted, so use this only
on a network you trust (your own hotspot, your home, a lab you control). For anything
beyond that, use setup B or C.

VARA refuses to listen on a network address without a `--policy`, so a server cannot be
left open to everyone by accident.

### B. A server that stays on (a cloud machine, a Raspberry Pi, a lab server)

Use Docker, and put a web server that handles HTTPS in front of it. The steps are in
[DEPLOYMENT.md](DEPLOYMENT.md): "Run your own Hub" and "TLS & reverse proxies". This is the
setup to use if people are not in the same place or the server should be reachable all the
time.

### C. No server at all

A repository can be cloned from a folder, so a shared drive, a USB stick or a synced folder
can carry work between two people:

```sh
vara clone /media/usb/project-repo project
# ... work, commit ...
vara push        # writes back to the folder you cloned from
```

**Careful.** If someone is also working in that folder, a push moves the branch they have open
without updating their files, and `vara status` there still says "working tree clean". Their next
commit could undo what you pushed. Keep the shared folder for pushing and pulling only, and nobody
edits files in it. VARA does not yet refuse such a push, the way git does by default. Nobody can see
who changed what in real time, and two people pushing at once need care, so this suits two people
more than a team. For more than two, use a server.

## 2. Add people and share a repository

On the computer that runs the server (replace the folders with yours):

```sh
# sign in to your own server once, then make an account for Bob
vara login http://127.0.0.1:8737 ada --password 'your-password'
vara account create http://127.0.0.1:8737 bob --password 'a-first-password-for-bob'

# share the repository "notes" with him
vara share --policy ~/hub/policy --accounts ~/hub/accounts notes bob --write
```

(The first account is made with `--accounts` on the server computer, as in step 1. Further accounts
are made over the running server, as above, because the first-account form refuses to run a
second time.) Give Bob his first password in person, and ask him to change it after signing in:
`vara account passwd http://192.168.1.20:8737 bob --password 'his-own-password'`
(see `vara account --help`).

`--write` lets Bob clone, pull, push and create branches. `--read` lets him only read.
`--none` takes his access away again. It also lets him see the repository in the Hub's web
page, which used to need a second edit by hand.

A person you share with can **not** force-push, delete a branch, rename or delete the
repository, or change who has access. The owner's access is never changed by this command.

See who has access:

```sh
vara share --policy ~/hub/policy notes
```

```
notes
  ada                  owner
  bob                  write
```

Share only needs the server's own folders, so it runs on the server computer. It does not
use the network and does not ask for a login.

## 3. What your teammates do

Once, to install: download `vara` from the release page (or use the one-line installer in
[DEPLOYMENT.md](DEPLOYMENT.md)), then tell VARA who you are:

```sh
vara identity set --default --name "Bob" --email bob@example.com
```

Sign in once. VARA remembers the session, and does not print the secret:

```sh
vara login http://192.168.1.20:8737 bob --password 'bobs-password'
```

Clone, then work as usual:

```sh
vara clone http://192.168.1.20:8737/notes
cd notes
# edit files ...
vara add .
vara commit -m "Week 3 notes"
vara push
vara pull          # get what the others pushed
```

If two people changed the same lines, VARA marks the conflict and you choose, for example
`vara merge other-branch --interactive` to decide piece by piece.

## What this does not do yet

- **No hosted service.** Someone has to run the server.
- **The web page is read-only.** It shows files, history, differences and search. Pushing
  is done with `vara`.
- **No pull requests, issues or teams yet.** They are planned for v0.5. Today, people share
  one repository with `--write` and agree among themselves how to use branches.
- **Changing access is done on the server computer**, not from the web page.
- **No encryption on its own.** Use HTTPS in front of the server for anything beyond a
  network you trust.
