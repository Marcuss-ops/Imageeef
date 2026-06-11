#pragma once

#include <filesystem>
#include <string>
#include <vector>

namespace velox {

struct SceneRuntime {
    std::string text;
    std::string image_link;
    std::vector<std::string> image_links;
    double duration_seconds{0.0};
};

struct ClipRuntime {
    std::string text;
    std::string clip_link;
    std::vector<std::string> clip_links;
    double duration_seconds{0.0};
    std::string kind;
};

double extractDurationValue(const std::string& json, const std::string& key, double fallback);
ClipRuntime parseClipObject(const std::string& obj);
std::vector<SceneRuntime> parseScenes(const std::string& requestJson);
std::vector<ClipRuntime> parseClipSegments(const std::string& requestJson);
std::vector<std::string> parseStringListField(const std::string& requestJson, const std::string& key);
std::filesystem::path firstAvailableImage(const SceneRuntime& scene, const std::filesystem::path& workDir, size_t index);
std::filesystem::path firstAvailableClip(const std::vector<std::string>& candidates, const std::filesystem::path& workDir, size_t index);

} // namespace velox
