import argparse
import json
import logging
import sys

parser = argparse.ArgumentParser()
parser.add_argument('-payload', '--payload', help='Payload from JSM', required=True)
parser.add_argument('-apiKey', '--apiKey', help='API Key', required=True)
parser.add_argument('-jsmUrl', '--jsmUrl', help='JSM URL', required=True)
parser.add_argument('-logLevel', '--logLevel', help='logLevel', required=True)
parser.add_argument('-jecNamedPipe', '--jecNamedPipe', type=str, help="Path of Named Pipe to write callback context", required=True)
parser.add_argument('-example-configured-arg', '--example-configured-arg', help='example-configured-arg', required=False)
args = vars(parser.parse_args())

logging.basicConfig(stream=sys.stdout, level=args['logLevel'])

payload_string = args.get("payload")
payload_string = payload_string.strip()
payload = json.loads(payload_string)

def readValue(key):
	return payload.get(key)

def writeCallback(callbackJson):
	pipe_path = args.get("jecNamedPipe")
	with open(pipe_path, "w") as pipe:
		pipe.write(str(callbackJson))
		pipe.close()

testValue = readValue("test-key")

logging.info(testValue)

callback = '{"test-callback": "value"}'
callbackJson = json.loads(callback)

writeCallback(callbackJson)