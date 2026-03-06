#!/usr/bin/env python3
"""
Extract core inference API types from OpenAI OpenAPI spec.
Filters to only include Chat, Completions, Embeddings, Models, and Moderations.
"""

import yaml
import sys
import re
import copy

# Core schemas we need
CORE_SCHEMAS = {
    # Models
    'Model', 'ListModelsResponse',
    # Chat Completions
    'CreateChatCompletionRequest', 'CreateChatCompletionResponse',
    'ChatCompletionRequestMessage', 'ChatCompletionResponseMessage',
    'ChatCompletionChoice', 'ChatCompletionChunk',
    'ChatCompletionRequestUserMessage', 'ChatCompletionRequestAssistantMessage',
    'ChatCompletionRequestSystemMessage', 'ChatCompletionRequestDeveloperMessage',
    'ChatCompletionRequestToolMessage', 'ChatCompletionRequestFunctionMessage',
    'ChatCompletionRequestMessageContentPart', 'ChatCompletionRequestMessageContentPartText',
    'ChatCompletionRequestMessageContentPartImage', 'ChatCompletionRequestMessageContentPartAudio',
    'ChatCompletionRequestMessageContentPartFile',
    'ChatCompletionMessageToolCall', 'ChatCompletionFunctionCall',
    'ChatCompletionStreamOptions',
    # Completions (Legacy)
    'CreateCompletionRequest', 'CreateCompletionResponse',
    'CompletionChoice', 'Logprobs', 'CreateCompletionResponseChoice',
    # Embeddings
    'CreateEmbeddingRequest', 'CreateEmbeddingResponse',
    'EmbeddingData',
    # Moderations
    'CreateModerationRequest', 'CreateModerationResponse',
    'ModerationResult', 'ModerationCategories', 'CategoryScores',
    # Common
    'Usage', 'ErrorResponse', 'ErrorDetail',
    'Tool', 'ToolFunction', 'ResponseFormat',
    'ChatCompletionTokenLogprob', 'ChatCompletionStreamResponseChoice',
}

# Core paths
CORE_PATHS = [
    '/models',
    '/models/{model}',
    '/chat/completions',
    '/completions',
    '/embeddings',
    '/moderations',
]

def find_dependencies(schema_name, components, found=None):
    """Recursively find all schema dependencies."""
    if found is None:
        found = set()
    
    if schema_name in found:
        return found
    
    if schema_name not in components:
        return found
    
    found.add(schema_name)
    schema = components[schema_name]
    
    def extract_refs(obj, found):
        """Extract $ref values from nested object."""
        if isinstance(obj, dict):
            if '$ref' in obj:
                ref = obj['$ref']
                if ref.startswith('#/components/schemas/'):
                    dep_name = ref.split('/')[-1]
                    find_dependencies(dep_name, components, found)
            for v in obj.values():
                extract_refs(v, found)
        elif isinstance(obj, list):
            for item in obj:
                extract_refs(item, found)
    
    extract_refs(schema, found)
    return found

def fix_anyof_null(content):
    """Fix anyOf with null types for OpenAPI 3.0 compatibility."""
    # Replace OpenAPI 3.1 patterns with OpenAPI 3.0 compatible ones
    
    # Pattern: anyOf: [type: "null", $ref: ...] -> $ref + nullable: true
    content = re.sub(
        r'anyOf:\s*\n(\s*)-\s*type:\s*"null"\s*\n\s*-\s*\$ref:\s*[\'"]#/components/schemas/([^\'"]+)[\'"]',
        r'$ref: \'#/components/schemas/\2\'\n\1nullable: true',
        content
    )
    content = re.sub(
        r'anyOf:\s*\n(\s*)-\s*\$ref:\s*[\'"]#/components/schemas/([^\'"]+)[\'"]\s*\n\s*-\s*type:\s*"null"',
        r'$ref: \'#/components/schemas/\2\'\n\1nullable: true',
        content
    )
    
    # Pattern: anyOf: [type: "null", type: X] -> type: X, nullable: true
    content = re.sub(
        r'anyOf:\s*\n(\s*)-\s*type:\s*"null"\s*\n\s*-\s*type:\s*"(\w+)"',
        r'type: "\2"\n\1nullable: true',
        content
    )
    content = re.sub(
        r'anyOf:\s*\n(\s*)-\s*type:\s*"(\w+)"\s*\n\s*-\s*type:\s*"null"',
        r'type: "\2"\n\1nullable: true',
        content
    )
    
    # Change version to 3.0.3
    content = content.replace('openapi: 3.1.0', 'openapi: 3.0.3')
    
    return content

def clean_schema(schema):
    """Remove problematic OpenAPI 3.1 features."""
    if isinstance(schema, dict):
        result = {}
        for k, v in schema.items():
            # Skip OpenAPI 3.1 specific keywords
            if k in ('discriminator', 'externalDocs', 'example', 'deprecated'):
                continue
            # Recurse
            result[k] = clean_schema(v)
        return result
    elif isinstance(schema, list):
        return [clean_schema(item) for item in schema]
    else:
        return schema

def main():
    input_file = sys.argv[1] if len(sys.argv) > 1 else 'api/openai/openapi-full.yaml'
    output_file = sys.argv[2] if len(sys.argv) > 2 else 'api/openai/openapi-core.yaml'
    
    print(f"Reading {input_file}...")
    with open(input_file, 'r') as f:
        content = f.read()
    
    # Fix anyOf/null patterns for 3.0 compatibility
    print("Fixing OpenAPI 3.0 compatibility issues...")
    content = fix_anyof_null(content)
    
    spec = yaml.safe_load(content)
    
    components = spec.get('components', {}).get('schemas', {})
    
    # Find all required schemas
    all_schemas = set()
    for core_schema in CORE_SCHEMAS:
        find_dependencies(core_schema, components, all_schemas)
    
    print(f"Found {len(all_schemas)} schema dependencies")
    
    # Filter paths to core endpoints only
    core_paths = {}
    for pattern in CORE_PATHS:
        if pattern in spec.get('paths', {}):
            core_paths[pattern] = spec['paths'][pattern]
    
    print(f"Found {len(core_paths)} core paths")
    
    # Create filtered schemas
    filtered_schemas = {}
    for schema_name in sorted(all_schemas):
        if schema_name in components:
            filtered_schemas[schema_name] = clean_schema(components[schema_name])
    
    # Create filtered spec
    filtered_spec = {
        'openapi': '3.0.3',
        'info': {
            'title': 'OpenAI API (Core Inference)',
            'description': 'Core inference APIs extracted from OpenAI OpenAPI spec',
            'version': spec['info']['version'],
        },
        'servers': spec.get('servers', []),
        'paths': core_paths,
        'components': {
            'schemas': filtered_schemas,
        },
    }
    
    print(f"Writing {output_file}...")
    with open(output_file, 'w') as f:
        yaml.dump(filtered_spec, f, default_flow_style=False, sort_keys=False, allow_unicode=True)
    
    print(f"Done! Generated spec with {len(core_paths)} paths and {len(filtered_schemas)} schemas")

if __name__ == '__main__':
    main()