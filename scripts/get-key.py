#!python3
import requests
import getpass
import json
import os

RWC_ENDPOINT = os.getenv("RWC_ENDPOINT")
sa_name = 'terraform-provider'

if not RWC_ENDPOINT:
    RWC_ENDPOINT = "https://canary-useast2-acc.risingwave.cloud/api/v1"

email = input("Enter email: ")
password = getpass.getpass("Enter password: ")

# get token
response = requests.post(f"{RWC_ENDPOINT}/auth/login", json={
    "email": email,
    "password": password
})
if response.status_code == 200:
    token = response.json().get('tokens', {}).get('jwt', None)
    if not token:
        print("Failed to extract JWT token:", response.text)
        exit(1)
else:
    print("Failed to log in:", response.text)
    exit(1)

# get service accounts
principal = ""
response = requests.get(f"{RWC_ENDPOINT}/service-accounts", headers={
    "Authorization": f"Bearer {token}"
})
if response.status_code == 200:
    service_accounts = response.json().get('service_accounts', [])
    for sa in service_accounts:
        if sa['name'] == sa_name:
            principal = sa['id']
            break
else:
    print("Failed to get service accounts:", response.text)
    exit(1)

# create a service account
if len(principal) == 0:
    response = requests.post(f"{RWC_ENDPOINT}/service-accounts", headers={
        "Authorization": f"Bearer {token}"
    }, json={
        "name": sa_name,
        "description": "tf module service account, created by the started script for risingwavecloud provider"
    })
    if response.status_code == 200:
        principal = response.json().get('id', None)
        if not principal:
            print("Failed to extract service account ID", response.text)
            exit(1)
    else:
        print("Failed to create service account:", response.text)
        exit(1)

# create API key
response = requests.post(f"{RWC_ENDPOINT}/api-keys", headers={
    "Authorization": f"Bearer {token}"
}, json={
    "principal": principal,
    "description": "tf module API key, created by the started script for risingwavecloud provider"
})
if response.status_code == 200:
    b = response.json()
    api_key = b.get('key', None)
    if not api_key:
        print("Failed to extract API key", response.text)
        exit(1)
    api_secret = b.get('secret', None)
    if not api_secret:
        print("Failed to extract API secret", response.text)
        exit(1)
else:
    print("Failed to create API key:", response.text)
    exit(1)

print("\nAn API key has been created. Please note that your API secret is displayed only once, so make sure to remember it.\n")
print("API key   :", api_key)
print("API secret:", api_secret)
