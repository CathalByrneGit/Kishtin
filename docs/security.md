# Keeping the Kishtin computer safe

Kishtin keeps everything on one computer in your restaurant. That keeps your records yours, but it means the computer itself needs looking after. This page lists what to do, in order of importance. Most of it is done once, in under an hour.

It follows the five controls of [Cyber Essentials](https://www.ncsc.gov.uk/cyberessentials/overview), the UK's basic security scheme, which suit a small business anywhere: firewall, secure settings, who can get in, protection from malware, and updates. Doing what is here does not certify you for Cyber Essentials or anything else. Ireland's [National Cyber Security Centre](https://www.ncsc.gov.ie/) has more advice for small businesses.

## What is on the computer

Kishtin's data folder (`C:\Users\<you>\.kishtin` on Windows, `.kishtin` in your home folder on a Mac or Linux) holds:

- the database: HACCP records, staff names and their PINs and passwords (scrambled, not readable), orders, and customers' names and order notes until they are cleared ([30 days by default](recording.md#customers-names-and-order-notes));
- photos of delivery dockets and paper records, and voice recordings if you turned them on;
- backups of the database;
- Kishtin's certificate authority, which tablets trust;
- the access tokens for Square or SumUp, if you connected a till.

Anyone who can log in to the computer as the person who installed Kishtin, or who takes the computer or its disk, can read all of it. The steps below stop that.

## 1. Turn on disk encryption

**Most important.** If the computer is stolen, encryption means nobody can read the disk without your password.

- **Windows 11 Pro, Windows 10 Pro**: Start, type **BitLocker**, open **Manage BitLocker**, choose **Turn on BitLocker** for the C: drive.
- **Windows 11 Home**: **Settings › Privacy & security › Device encryption**, turn it on. If you don't see it, the computer does not support it: ask whoever supplied it, or upgrade to Pro for BitLocker.
- **Mac**: **System Settings › Privacy & Security › FileVault**, choose **Turn On**.
- **Linux**: choose disk encryption when installing. Adding it later usually means reinstalling.

Windows and the Mac give you a **recovery key**. Without it, a forgotten password or a broken computer can mean losing everything on it. Print it or write it down, and keep it somewhere safe that is not with the computer, such as at home.

What it changes for Kishtin:

- **Windows**: nothing. BitLocker unlocks by itself when the computer starts, and Kishtin starts when someone logs in, as before.
- **Mac**: with FileVault on, a Mac does not log in by itself. After a power cut, someone has to log in before Kishtin starts.
- **Linux**: after a power cut, someone has to type the disk password before the computer, and Kishtin, starts.

## 2. Give Kishtin its own computer account

Use the Kishtin computer for Kishtin, not for email or browsing: most attacks start with an email or a web page.

- Make a **standard (not administrator) account** for Kishtin, with its own strong password, and install Kishtin while logged in as it. Kishtin does not need an administrator account.
- Keep a separate administrator account for installing updates, with a different password.
- Turn on the **screen lock** after a few minutes. Kishtin keeps running when the screen is locked.
- Don't share the account's password with staff. Staff use Kishtin from the tablets with their own PIN.

## 3. Firewall and network

- Keep the computer's **firewall turned on** (on Windows and a Mac it is on by default).
- When Windows asks whether to allow `kishtind`, allow it on **private networks only**. A Mac asks whether to accept incoming connections: allow it.
- Keep the Kishtin computer and the tablets on the **staff Wi-Fi**, never the guest Wi-Fi. Customers' phones should not reach the kitchen system at all.
- Never forward Kishtin's ports (8080, 8443) on the router to the internet. Kishtin is made to be reached from inside the restaurant only.
- Change the router's own admin password from the one printed on it.

## 4. Who can get into Kishtin

- **Everyone has their own PIN.** Records show who did each check, so a shared PIN makes them worthless.
- **Have two managers with passwords**, so one forgotten password does not lock you out (see [Forgotten manager password](install.md#forgotten-manager-password)).
- **When someone leaves**, deactivate them in **Setup › Staff and access › Staff** the same day, and anonymise them when you no longer need their name in the logs (see [Leavers](recording.md#leavers)).
- **If a tablet or phone is lost**, revoke its key in **Setup › Staff and access › Device keys** straight away. Nobody can use Kishtin from it after that.
- Give only the permissions people need (**Setup › Staff and access › Permissions**).

## 5. Protection from malware

- **Windows**: keep **Microsoft Defender** (Windows Security) turned on. It is free and built in. Another antivirus product can replace it; don't run two.
- **Mac**: keep **System Settings › Privacy & Security** allowing apps from the App Store and identified developers only. The install line works with this setting.
- Only install Kishtin with the install line in [Installing Kishtin](install.md), which checks the download before installing.
- Don't plug unknown USB sticks into the Kishtin computer.

## 6. Updates

- Turn on **automatic updates** for the computer's operating system.
  - **Windows**: **Settings › Windows Update**, then **Advanced options › Active hours**, and set your service hours so it never restarts during service.
  - **Mac**: **System Settings › General › Software Update › Automatic updates**.
- Update **Kishtin** when a new release comes out: run the install line again (see [Update](install.md#update)). Only the latest release gets security fixes.
- A computer that no longer gets updates from Microsoft or Apple should be replaced.

## 7. Backups

Kishtin backs up the database every 6 hours, on the same computer. That doesn't help if the computer is stolen, breaks or catches fire.

- **Every week**, copy the `backups` and `files` folders from Kishtin's data folder to a USB stick or another computer. See **Setup › Backups** for where they are.
- **Encrypt the USB stick**: on Windows right-click it and choose **Turn on BitLocker** (BitLocker To Go, Pro editions), or on a Mac erase it in **Disk Utility** choosing an **Encrypted** format. Backups hold everything listed above except the till tokens and the certificate authority.
- Keep the stick away from the computer, and use two in turn, so a fire or theft doesn't take both.
- Old backups still hold customers' names and notes from before they were cleared. Delete old copies you no longer need.
- **Setup › Backups** shows each backup on the computer with its check result. Once in a while, try restoring a copy on another computer (below).

## Restoring from a backup

On a new computer, or after the old one is repaired:

1. [Install Kishtin](install.md), but say **no** to starting with the computer for now, and don't start it.
2. Make Kishtin's data folder: `C:\Users\<you>\.kishtin` on Windows, `.kishtin` in your home folder on a Mac or Linux.
3. Copy the latest backup (a file such as `kishtin-20261102T013047Z.db`) into it and rename it `kishtin.db`. Copy the `files` folder in beside it.
4. Start Kishtin (`kishtind`) and log in as before. **Exports › Verify** checks the records and photos came back whole.
5. Turn on starting with the computer: `kishtind startup on`.
6. Connect each tablet again with a QR code. The new computer has a new certificate, so tablets must trust it again.
7. Connect the till again in **Setup › Connections**: till tokens are never in backups.

Everything in the backup comes back: records, staff and their PINs, the menu, orders and settings. Anything logged after the backup was made is lost, which is why backups are copied off every week.

## If the computer is lost or stolen

1. If a till is connected, disconnect Kishtin from Square or SumUp in their own dashboard, which stops the old token working.
2. If the disk was **not encrypted**, staff and customer details may have been exposed. Under GDPR you may have to tell the [Data Protection Commission](https://www.dataprotection.ie/) within 72 hours. Ask them, or a solicitor, if you are not sure.
3. Restore the latest backup you copied off onto a new computer (above).
4. Then set new manager passwords and staff PINs in **Setup › Staff and access › Staff**, and revoke the old device keys in **Setup › Staff and access › Device keys**: whoever has the old computer has the old database.

## What Kishtin does for you

So you know what is already handled:

- The API and screens need a key for every request. Tablets connect over an encrypted connection (HTTPS) and must be paired with a one-time code from a manager.
- Kishtin listens only on the computer itself, except for tablets on port 8443, and port 8080 shows other devices only the page that helps a tablet join.
- HACCP records cannot be changed, and **Exports › Verify** shows if anyone tampered with the database file.
- Till tokens are kept in a file only Kishtin's account can read, and never go into backups or exports.
- Nothing is sent off the computer unless a manager connects a till.

To report a security problem in Kishtin itself, see [SECURITY.md](https://github.com/CathalByrneGit/Kishtin/blob/main/SECURITY.md).
