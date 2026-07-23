# Mini Program Home Carousel Design

## Goal

Add a new admin parent menu named “小程序管理” with a child page named “首页管理”, allowing administrators to upload and manage images that appear as an auto-playing carousel at the very top of the mini-program home page.

## User Experience

The admin page presents a simple ordered list of carousel images. Administrators can add an image, replace it through the existing uploader, enable or disable it, move it up or down, delete it, and save. The mini program renders only enabled items with valid image URLs. When there are no enabled items, the home page keeps its current layout without an empty placeholder.

The carousel appears above the current brand/navigation row. It uses the native uni-app `swiper`, advances automatically every four seconds, loops continuously, shows indicator dots, and supports manual horizontal swiping. Images use `aspectFill` inside the existing rounded Apple-style visual system.

## Data Model

Store the configuration under the existing site configuration document:

```json
{
  "home": {
    "miniappCarousel": {
      "autoplay": true,
      "interval": 4000,
      "items": [
        {
          "image": "/api/upload-assets/123",
          "enabled": true
        }
      ]
    }
  }
}
```

Order is represented by array order. The public site-config endpoint already recursively rewrites referenced uploads into public asset paths. The mini program resolves API-relative public asset paths against its configured API host before rendering.

## Components

- Backend menu seed: add catalog `小程序管理` and menu `首页管理`.
- Admin route component: `views/miniapp/home.vue`, reusing `ImagePathInput` and the site-config editor API.
- Mini-program normalization utility: accept missing or malformed configuration safely, filter disabled/empty items, and resolve public image URLs.
- Mini-program home page: load cached configuration immediately, refresh in the background, and render the top carousel only when items exist.

## Error Handling

- Upload errors continue to use the existing uploader feedback.
- Save errors use the existing request error handling.
- Mini-program configuration failures leave the current home page intact.
- Individual image load failures hide only the failed image from the active carousel state.

## Verification

- Menu seed tests cover the new parent and child metadata.
- Admin source tests cover upload, ordering, enable/disable, deletion, and save wiring.
- Mini-program utility tests cover filtering and URL resolution.
- Mini-program UI compatibility tests cover top placement and native swiper settings.
- Backend/admin tests and both admin and mini-program builds pass.
