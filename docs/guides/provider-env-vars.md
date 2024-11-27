In AWS, you use the .aws folder to store your credentials and configuration in two main files:
	•	credentials: Stores access keys for different profiles.
	•	config: Stores region and output format preferences for profiles.

Each profile is referenced by its name, allowing you to switch between configurations easily. For example:

~/.aws/credentials
[default]
aws_access_key_id = YOUR_ACCESS_KEY
aws_secret_access_key = YOUR_SECRET_KEY

[profile_name]
aws_access_key_id = YOUR_OTHER_ACCESS_KEY
aws_secret_access_key = YOUR_OTHER_SECRET_KEY

Now let’s explore the equivalents for Google Cloud and Azure:

Google Cloud

In Google Cloud, the equivalent method uses the gcloud CLI and stores credentials in a directory called ~/.config/gcloud:
	1.	Authentication File:
	•	You authenticate using gcloud auth login or gcloud auth application-default login, which generates a file called application_default_credentials.json stored in ~/.config/gcloud/.
	2.	Configuration Profiles:
	•	Google Cloud uses “configurations” to manage multiple profiles. These are stored in the configurations folder within ~/.config/gcloud/.
	•	Switch between configurations using:

gcloud config configurations activate <configuration_name>


	•	A configuration includes settings like project ID, region, and zone.
Example:

gcloud config set project my-project-id
gcloud config set compute/region us-central1


	3.	Service Account Credentials:
	•	Service account keys can be explicitly set by exporting the GOOGLE_APPLICATION_CREDENTIALS environment variable:

export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"

Azure

In Azure, the equivalent method revolves around the Azure CLI and az configuration, with credentials stored in specific locations or handled via environment variables:
	1.	Authentication:
	•	Log in using az login, which caches authentication tokens in ~/.azure/ (e.g., ~/.azure/accessTokens.json).
	•	For service principals, use:

az login --service-principal --username <appId> --password <password> --tenant <tenant>


	2.	Profiles and Contexts:
	•	Azure CLI doesn’t have the concept of named profiles like AWS, but it supports managing subscriptions, which are akin to profiles:

az account set --subscription "<subscription-id>"


	•	The current subscription acts as the “profile” in this sense.

	3.	Environment Variables:
	•	For automation, you can use environment variables to set service principal credentials:

export AZURE_CLIENT_ID=<appId>
export AZURE_SECRET=<password>
export AZURE_TENANT=<tenant>


	4.	Configuration File:
	•	Azure CLI configurations are stored in ~/.azure/:
	•	config: Stores default settings like output format and default location.
	•	tokens.json: Stores authentication tokens.

Summary Table

Feature	AWS .aws Folder	Google Cloud ~/.config/gcloud	Azure ~/.azure Folder
Credentials File	credentials, config	application_default_credentials.json	accessTokens.json, Environment Variables
Profiles/Contexts	Named profiles in ~/.aws	Named configurations in gcloud	Subscription selection via az
Environment Vars	AWS_PROFILE, keys explicitly	GOOGLE_APPLICATION_CREDENTIALS	AZURE_CLIENT_ID, etc.

Each cloud provider has its unique approach to handling multiple profiles and credentials, but all three can be adapted for multi-environment workflows.
