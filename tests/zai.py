from openai import OpenAI
from dotenv import load_dotenv
from os import getenv
load_dotenv()
client = OpenAI(
    base_url="https://api.z.ai/api/coding/paas/v4",
    api_key=getenv("API_KEY_ZAI"),
)
completion = client.chat.completions.create(
    model="glm-4.7",
    messages=[{"role": "user", "content": "Write a 100 words history about pirates and computer in the space"}],
    temperature=1,
    top_p=0.95,
    max_tokens=16384,
    extra_body={"chat_template_kwargs": {"enable_thinking": True}, "reasoning_budget": 16384},
    stream=True
)
for chunk in completion:
    if not chunk.choices:
        continue
    reasoning = getattr(chunk.choices[0].delta, "reasoning_content", None)
    if reasoning:
        print(reasoning, end="")
    if chunk.choices[0].delta.content is not None:
        print(chunk.choices[0].delta.content, end="")
