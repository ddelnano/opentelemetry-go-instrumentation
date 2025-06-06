import argparse
import json
import sys
import os

def transform_offset_results_flat(input_path, output_path):
    with open(input_path, 'r') as f:
        data = json.load(f)

    # This will hold the final transformed data.
    #
    # Example structure of "result" when finished:
    # {
    #   "golang.org/x/net/http2.DataFrame": {
    #       "data": {
    #           "0.1.0": null,
    #           "0.2.0": null,
    #           ...
    #       }
    #   },
    #   "golang.org/x/net/http2.FrameHeader": {
    #       "Flags": {
    #           "0.1.0": 2,
    #           ...
    #       },
    #       ...
    #   },
    #   "crypto/tls.(*Conn).Read": {
    #       "b": {
    #           "1.19.0": {
    #               "location": "registers",
    #               "offset": 8
    #           },
    #           "1.19.1": ...
    #       },
    #       "c": {...},
    #   }
    #   ...
    # }
    #
    # Each of those top-level keys aggregates the fields (for structs)
    # or arguments (for funcs) under it.

    result = {}

    for module_entry in data:
        # e.g. "golang.org/x/net"
        module_name = module_entry["module"]

        for pkg in module_entry["packages"]:
            # e.g. "golang.org/x/net/http2"
            package_name = pkg["package"]

            # Handle all structs
            if "structs" in pkg and pkg["structs"]:
                for st in pkg["structs"]:
                    struct_name = st["struct"]  # e.g. "DataFrame"
                    struct_key = f"{package_name}.{struct_name}"
                    # Ensure we have an entry for this struct
                    if struct_key not in result:
                        result[struct_key] = {}

                    # Each struct can have multiple fields
                    for field in st["fields"]:
                        field_name = field["field"]
                        if field_name not in result[struct_key]:
                            result[struct_key][field_name] = {}

                        # Each field can have multiple “offset” groups, each specifying certain versions
                        for off_obj in field["offsets"]:
                            offset_val = off_obj["offset"]
                            location_val = off_obj.get("location", None)

                            # Assign this offset (or offset object) to all listed versions
                            for ver in off_obj["versions"]:
                                if location_val and location_val.lower() != "unknown":
                                    # Store as an object with location + offset
                                    result[struct_key][field_name][ver] = {
                                        "location": location_val,
                                        "offset": offset_val
                                    }
                                else:
                                    # If no special location, just store the offset (possibly null)
                                    result[struct_key][field_name][ver] = offset_val

            # Handle all funcs
            if "funcs" in pkg and pkg["funcs"]:
                for fn in pkg["funcs"]:
                    func_name = fn["func"]  # e.g. "(*Encoder).WriteField"
                    func_key = f"{package_name}.{func_name}"
                    # Ensure we have an entry for this function
                    if func_key not in result:
                        result[func_key] = {}

                    # Each function can have multiple arguments
                    for arg in fn["args"]:
                        arg_name = arg["arg"]
                        if arg_name not in result[func_key]:
                            result[func_key][arg_name] = {}

                        for off_obj in arg["offsets"]:
                            offset_val = off_obj["offset"]
                            location_val = off_obj.get("location", None)

                            for ver in off_obj["versions"]:
                                if location_val and location_val.lower() != "unknown":
                                    result[func_key][arg_name][ver] = {
                                        "location": location_val,
                                        "offset": offset_val
                                    }
                                else:
                                    result[func_key][arg_name][ver] = offset_val

    # Write the final output as JSON
    with open(output_path, 'w') as out_f:
        json.dump(result, out_f, indent=2)


def main():
    parser = argparse.ArgumentParser(
        description="Transform offset_results.json into either a flat or nested format."
    )
    parser.add_argument("input_file", help="Path to the input JSON file (offset_results.json).")
    parser.add_argument("output_file", help="Path to the transformed output JSON file.")
    parser.add_argument(
        "--mode",
        choices=["flat", "nested"],
        default="flat",
        help="Select the transformation mode: 'flat' for top-level keys, 'nested' for top-level 'structs' and 'funcs'."
    )

    args = parser.parse_args()

    if args.mode == "nested":
        transform_offset_results_nested(args.input_file, args.output_file)
        print(f"Transformed (nested) JSON written to {args.output_file}")
    else:
        transform_offset_results_flat(args.input_file, args.output_file)
        print(f"Transformed (flat) JSON written to {args.output_file}")

def transform_offset_results_nested(input_path, output_path):
    with open(input_path, 'r') as f:
        data = json.load(f)

    # We'll separate everything into top-level "structs" and "funcs"
    result = {
        "structs": {},
        "funcs": {}
    }

    for module_entry in data:
        for pkg in module_entry["packages"]:
            package_name = pkg["package"]

            # Process structs
            if "structs" in pkg and pkg["structs"]:
                for st in pkg["structs"]:
                    struct_name = st["struct"]
                    struct_key = f"{package_name}.{struct_name}"

                    if struct_key not in result["structs"]:
                        result["structs"][struct_key] = {}

                    for field in st["fields"]:
                        field_name = field["field"]
                        if field_name not in result["structs"][struct_key]:
                            result["structs"][struct_key][field_name] = {}

                        for off_obj in field["offsets"]:
                            offset_val = off_obj["offset"]
                            location_val = off_obj.get("location", None)

                            for ver in off_obj["versions"]:
                                if location_val and location_val.lower() != "unknown":
                                    result["structs"][struct_key][field_name][ver] = {
                                        "location": location_val,
                                        "offset": offset_val
                                    }
                                else:
                                    result["structs"][struct_key][field_name][ver] = offset_val

            # Process funcs
            if "funcs" in pkg and pkg["funcs"]:
                for fn in pkg["funcs"]:
                    func_name = fn["func"]
                    func_key = f"{package_name}.{func_name}"

                    if func_key not in result["funcs"]:
                        result["funcs"][func_key] = {}

                    for arg in fn["args"]:
                        arg_name = arg["arg"]
                        if arg_name not in result["funcs"][func_key]:
                            result["funcs"][func_key][arg_name] = {}

                        for off_obj in arg["offsets"]:
                            offset_val = off_obj["offset"]
                            location_val = off_obj.get("location", None)

                            for ver in off_obj["versions"]:
                                if location_val and location_val.lower() != "unknown":
                                    result["funcs"][func_key][arg_name][ver] = {
                                        "location": location_val,
                                        "offset": offset_val
                                    }
                                else:
                                    result["funcs"][func_key][arg_name][ver] = offset_val

    with open(output_path, 'w') as out_f:
        json.dump(result, out_f, indent=2)

if __name__ == "__main__":
    main()

