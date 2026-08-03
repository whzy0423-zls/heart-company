import { createHashRouter, Navigate } from "react-router-dom";

import CanvasPage from "@/pages/canvas";
import CanvasProjectPage from "@/pages/canvas/project";
import { canvasRoutePaths } from "@/router-config";

export const router = createHashRouter([
    { path: canvasRoutePaths[0], element: <CanvasPage /> },
    { path: canvasRoutePaths[1], element: <CanvasProjectPage /> },
    { path: "*", element: <Navigate to={canvasRoutePaths[0]} replace /> },
]);
