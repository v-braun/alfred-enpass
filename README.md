# alfred-enpass
> Search Enpass via the Alfred App


By [v-braun - viktor-braun.de](https://viktor-braun.de).


<p align="center">
<img width="70%" src="resources/preview.gif" />
</p>


## Description
Search wihin Enpass for Logins.

Unfortunatley Enpass does not provide any APIs for developers.
So this workflow is accessing your vaults DB file directley.

To setup this workflow, you need to provide once this path and enter your master password.

The Password is not stored in clear text, it is persistent within the keychain app.
This is why you may need to enter your password "sometimes".


## Installation

Download the workflow [here](https://github.com/v-braun/alfred-enpass/raw/main/workflow-releases/Search%20Enpass.alfredworkflow)

On First run you need to speficy the **path** to the vault (folder not file).
You can find it in Enpass under *Settings -> Advanced -> Data Location*.
If you open this path in Finder you will see one folder per Vault.
Go inside of this folder and copy the path of it.

Usually this is somewhere here:  
~/Library/Containers/in.sinew.Enpass-Desktop/Data/Documents/**VAULT_NAME_HERE**


Paste this location into Alfred on first run.

Afterwords you will be asked to enter your masterpassword.  
This password will be stored in your Keychain, for that you may need to unlock your keychain during setup and (depends on your keychain settings) when the keychain is locked again and you trigger the Workflow.


## Authors

![image](https://avatars3.githubusercontent.com/u/4738210?v=3&amp;s=50)  
[v-braun](https://github.com/v-braun/)



## License
**alfred-enpass** is available under the MIT License. See [LICENSE](LICENSE) for details.


## Thanks to
💜 [Alfred App](https://www.alfredapp.com/)  
💙 [Enpass](https://www.enpass.io/)  
💚 [icons8](https://icons8.com)  